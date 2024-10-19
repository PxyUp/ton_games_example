package game

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/PxyUp/ton_games_example/games"
	"github.com/PxyUp/ton_games_example/pkg/random"
	"github.com/google/uuid"
)

type State int8

type game struct {
	cfg     *games.MoreLessConfig
	updates chan games.GameEvent
	players map[string]games.Player
	id      string

	createdTime time.Time
	mutex       sync.Mutex

	ticker *time.Ticker

	gameCtx  context.Context
	gameStop context.CancelFunc
	finished bool
	creator  string
}

func (g *game) AddPlayerWithAction(player games.Player, event games.PlayerEvent) error {
	return games.ErrInvalidAction
}

func (g *game) SendUserEvent(_ games.PlayerEvent) error {
	return games.ErrInvalidAction
}

func (g *game) GetMaxPlayers() uint8 {
	return g.cfg.NumberOfPlayers
}

func (g *game) GameType() games.GameType {
	return games.MoreLess
}

func (g *game) RemovePlayer(pp games.Player) error {
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

func (g *game) AddPlayer(p games.Player) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if g.creator == p.GetId() {
		return games.ErrCreatorCantLeft
	}

	if g.finished {
		return games.ErrGameFinished
	}

	if _, ok := g.players[p.GetId()]; !ok {
		if len(g.players) >= int(g.cfg.NumberOfPlayers) {
			return games.ErrMaxPlayer
		}

		g.players[p.GetId()] = p
		g.updates <- games.NewGameEvent(g.GetID(), games.PlayerJoin, fmt.Sprintf("player: %s join game", p.GetId()), true, []games.Player{p}, nil)
	}

	return nil
}

func (g *game) Start(ctx context.Context) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.ticker == nil {
		g.ticker = time.NewTicker(g.cfg.Duration)
		g.gameCtx, g.gameStop = context.WithCancel(ctx)
		go g.loop()
	}

	return nil
}

func (g *game) getPlayers(withMutex bool) []games.Player {
	if withMutex {
		g.mutex.Lock()
		defer g.mutex.Unlock()
	}

	pl := make([]games.Player, len(g.players))
	i := 0
	for _, k := range g.players {
		pl[i] = k
		i += 1
	}

	return pl
}

func (g *game) loop() {
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

		if len(g.players) == 1 && len(winners) == 1 && winners[0].GetId() == g.creator {
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

func (g *game) GetID() string {
	return g.id
}

func (g *game) GetCost() float64 {
	return g.cfg.Cost
}

func (g *game) Abort() error {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.ticker == nil {
		return nil
	}

	g.ticker.Stop()
	g.gameStop()
	return nil
}

type playerNumber struct {
	Number   uint64 `json:"number"`
	PlayerID string `json:"player_id"`
}

func (g *game) GetWinners() ([]games.Player, games.GameMD, error) {
	maxNumber := uint64(0)

	players := g.getPlayers(false)

	pNumbers := make([]uint64, len(players))

	for i := range players {
		newRandom := random.GetRandom(uint64(g.cfg.MaxRandom))
		pNumbers[i] = newRandom
		if maxNumber <= newRandom {
			maxNumber = newRandom
		}
	}

	var winners []games.Player

	for i := range players {
		if pNumbers[i] == maxNumber {
			winners = append(winners, players[i])
		}
	}

	allNumbers := make([]*playerNumber, len(players))

	for i := range players {
		allNumbers[i] = &playerNumber{
			Number:   pNumbers[i],
			PlayerID: players[i].GetId(),
		}
	}

	return winners, map[string]interface{}{
		"max_number":     maxNumber,
		"player_numbers": allNumbers,
	}, nil
}

func (g *game) GetCreator() string {
	return g.creator
}

func (g *game) Updates() <-chan games.GameEvent {
	return g.updates
}

func (g *game) GetDuration() time.Duration {
	return g.cfg.Duration
}

func New(cfg *games.MoreLessConfig, creator string) games.Game {
	return &game{
		id:          uuid.New().String(),
		cfg:         cfg,
		updates:     make(chan games.GameEvent),
		createdTime: time.Now(),
		creator:     creator,
		players: map[string]games.Player{
			creator: &games.BasePlayer{
				Id: creator,
			},
		},
	}
}
