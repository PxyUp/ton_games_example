package rock_paper_scissors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/PxyUp/ton_games_example/games"
	"github.com/google/uuid"
)

var (
	_ games.Game = &spsGame{}

	ErrIncorrectGame = errors.New("incorrect game")
)

type Choice uint8

const (
	Rock Choice = iota
	Paper
	Scissors
)

func findWinners(players []*playerWithAction) []games.Player {
	if len(players) == 1 {
		return []games.Player{&games.BasePlayer{
			Id: players[0].player.GetId(),
		}}
	}

	choiceCounts := make(map[Choice]int)
	for _, player := range players {
		choiceCounts[player.choice]++
	}

	// If all choices are the same or there is one of each, it's a draw
	if len(choiceCounts) == 1 || len(choiceCounts) == 3 {
		allPlayers := make([]games.Player, len(players))
		for i, pp := range players {
			allPlayers[i] = &games.BasePlayer{
				Id: pp.player.GetId(),
			}
		}
		return allPlayers // Indicates a draw
	}

	// Identify the dominant choice (the one that wins against all others)
	var dominantChoice Choice
	for choice := range choiceCounts {
		if (choice == Rock && choiceCounts[Scissors] > 0 && choiceCounts[Paper] == 0) ||
			(choice == Scissors && choiceCounts[Paper] > 0 && choiceCounts[Rock] == 0) ||
			(choice == Paper && choiceCounts[Rock] > 0 && choiceCounts[Scissors] == 0) {
			dominantChoice = choice
			break
		}
	}

	// Find players with the dominant choice
	winners := []games.Player{}
	for _, player := range players {
		if player.choice == dominantChoice {
			winners = append(winners, &games.BasePlayer{
				Id: player.player.GetId(),
			})
		}
	}

	return winners
}

type playerWithAction struct {
	player games.Player
	choice Choice
}

type spsGame struct {
	id      string
	cfg     *games.RockPaperConfig
	players map[string]*playerWithAction

	updates chan games.GameEvent

	createdTime time.Time
	mutex       sync.Mutex

	ticker *time.Ticker

	gameCtx  context.Context
	gameStop context.CancelFunc
	finished bool
	creator  string
}

func (g *spsGame) AddPlayer(player games.Player) error {
	return games.ErrInvalidAction
}

func getActon(event games.PlayerEvent) (*PlayerChoiceEvent, error) {
	choiceEvent := &PlayerChoiceEvent{}
	errEvent := json.Unmarshal(event.GetRawData(), choiceEvent)
	if errEvent != nil {
		return nil, games.ErrInvalidAction
	}

	return choiceEvent, nil
}

func (g *spsGame) AddPlayerWithAction(player games.Player, event games.PlayerEvent) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if g.creator == player.GetId() {
		return games.ErrCreatorCantLeft
	}

	if g.finished {
		return games.ErrGameFinished
	}

	if _, ok := g.players[player.GetId()]; !ok {
		if len(g.players) >= int(g.cfg.NumberOfPlayers) {
			return games.ErrMaxPlayer
		}

		choiceEvent, errEvent := getActon(event)
		if errEvent != nil {
			return errEvent
		}

		g.players[player.GetId()] = &playerWithAction{
			player: player,
			choice: choiceEvent.Choice,
		}
		g.updates <- games.NewGameEvent(g.GetID(), games.PlayerJoin, fmt.Sprintf("player: %s join game", player.GetId()), true, []games.Player{player}, nil)
	}

	return nil
}

func (g *spsGame) GetID() string {
	return g.id
}

func (g *spsGame) GetCost() float64 {
	return g.cfg.Cost
}

func (g *spsGame) GameType() games.GameType {
	return games.RockPaperScissors
}

func (g *spsGame) GetDuration() time.Duration {
	return g.cfg.Duration
}

func (g *spsGame) RemovePlayer(pp games.Player) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if g.finished {
		return games.ErrGameFinished
	}

	if g.creator == pp.GetId() {
		return games.ErrCreatorCantLeft
	}

	if _, ok := g.players[pp.GetId()]; ok {
		delete(g.players, pp.GetId())
		g.updates <- games.NewGameEvent(g.GetID(), games.PlayerLeft, fmt.Sprintf("player: %s left game", pp.GetId()), true, []games.Player{pp}, nil)
	}

	return nil
}

func (g *spsGame) Start(ctx context.Context) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.ticker == nil {
		g.ticker = time.NewTicker(g.cfg.Duration)
		g.gameCtx, g.gameStop = context.WithCancel(ctx)
		go g.loop()
	}

	return nil
}

func (g *spsGame) loop() {
	defer func() {
		close(g.updates)
		g.gameStop()
	}()

	g.updates <- games.NewGameEvent(g.GetID(), games.Start, "game is started", true, g.getPlayers(true), nil)

	select {
	case <-g.gameCtx.Done():
		g.mutex.Lock()
		g.finished = true
		g.updates <- games.NewGameEvent(g.GetID(), games.Abort, "game is canceled", true, g.getPlayers(true), nil)
		g.mutex.Unlock()
		return
	case <-g.ticker.C:
		g.mutex.Lock()

		g.finished = true
		winners, md, err := g.GetWinners()
		if err != nil {
			g.updates <- games.NewGameEvent(g.GetID(), games.Error, "cant get game winners", true, g.getPlayers(false), md)
			g.mutex.Unlock()
			return
		}

		if (len(g.players) == 1 && len(winners) == 1 && winners[0].GetId() == g.creator) || (len(winners) == len(g.players)) {
			g.updates <- games.NewGameEvent(g.GetID(), games.NoWinners, "no winners, money back", true, g.getPlayers(false), md)
			g.updates <- games.NewGameEvent(g.GetID(), games.Finished, "game is finished", true, g.getPlayers(false), nil)
			g.mutex.Unlock()
			return
		}

		g.updates <- games.NewGameEvent(g.GetID(), games.Winners, "winners reveals", true, winners, md)
		g.updates <- games.NewGameEvent(g.GetID(), games.Finished, "game is finished", true, g.getPlayers(false), nil)
		g.mutex.Unlock()
	}
}

func (g *spsGame) getPlayers(withMutex bool) []games.Player {
	if withMutex {
		g.mutex.Lock()
		defer g.mutex.Unlock()
	}

	pl := make([]games.Player, len(g.players))
	i := 0
	for _, k := range g.players {
		pl[i] = k.player
		i += 1
	}

	return pl
}

func (g *spsGame) Abort() error {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.ticker == nil {
		return nil
	}

	g.ticker.Stop()
	g.gameStop()
	return nil
}

type playerChoice struct {
	Choice   Choice `json:"choice"`
	PlayerID string `json:"player_id"`
}

func (g *spsGame) GetWinners() ([]games.Player, games.GameMD, error) {
	players := make([]*playerWithAction, len(g.players))

	i := 0
	for _, p := range g.players {
		players[i] = p
		i += 1
	}

	if len(players) < 1 {
		return nil, nil, ErrIncorrectGame
	}

	mdChoices := make([]*playerChoice, len(players))

	for index, pp := range players {
		mdChoices[index] = &playerChoice{
			Choice:   pp.choice,
			PlayerID: pp.player.GetId(),
		}
	}

	winners := findWinners(players)
	arrWinners := make([]string, len(winners))
	for index, player := range winners {
		arrWinners[index] = player.GetId()
	}

	md := map[string]interface{}{
		"players": mdChoices,
		"winners": arrWinners,
	}

	return winners, md, nil
}

func (g *spsGame) Updates() <-chan games.GameEvent {
	return g.updates
}

func (g *spsGame) SendUserEvent(event games.PlayerEvent) error {
	return games.ErrInvalidAction
}

func (g *spsGame) GetCreator() string {
	return g.creator
}

func (g *spsGame) GetMaxPlayers() uint8 {
	return g.cfg.NumberOfPlayers
}

func New(cfg *games.RockPaperConfig, creator string, action games.PlayerEvent) (games.Game, error) {
	choiceEvent, errEvent := getActon(action)
	if errEvent != nil {
		return nil, errEvent
	}

	return &spsGame{
		id:          uuid.New().String(),
		cfg:         cfg,
		updates:     make(chan games.GameEvent),
		createdTime: time.Now(),
		creator:     creator,
		players: map[string]*playerWithAction{
			creator: {
				player: &games.BasePlayer{
					Id: creator,
				},
				choice: choiceEvent.Choice,
			},
		},
	}, nil
}
