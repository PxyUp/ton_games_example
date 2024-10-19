package database

import (
	"context"
	"time"

	"github.com/PxyUp/ton_games_example/games"
	"github.com/xssnick/tonutils-go/tlb"
)

var (
	_ GameRecord = &gameRecord{}
)

type GameRecord interface {
	GetId() string
	GetCost() int64
	GetState() games.GameState
	GetCreationTime() time.Time
	GetPlayers() []string
	GetMaxPlayers() uint8
	GetCreator() string
	JSON() map[string]interface{}
}

type historyRecord struct {
	Timestamp time.Time           `json:"timestamp"`
	HType     games.GameEventType `json:"type"`
	Msg       string              `json:"msg"`
	MD        games.GameMD        `json:"metadata"`
}

type gameRecord struct {
	id           string
	cost         int64
	state        games.GameState
	creationTime time.Time
	players      []string
	maxPlayers   uint8
	creator      string
	duration     time.Duration

	history  []*historyRecord
	gameType games.GameType
}

func (g *gameRecord) GetState() games.GameState {
	return g.state
}

func (g *gameRecord) GetId() string {
	return g.id
}

func (g *gameRecord) GetCreationTime() time.Time {
	return g.creationTime
}

func (g *gameRecord) GetPlayers() []string {
	return g.players
}

func (g *gameRecord) GetMaxPlayers() uint8 {
	return g.maxPlayers
}

func (g *gameRecord) GetGameType() games.GameType {
	return g.gameType
}

func (g *gameRecord) JSON() map[string]interface{} {
	timeLeft := int(time.Until(g.GetCreationTime().Add(g.duration)).Seconds())
	if timeLeft < 0 {
		timeLeft = 0
	}

	return map[string]interface{}{
		"id":            g.GetId(),
		"time_left":     timeLeft,
		"creation_time": g.GetCreationTime(),
		"cost":          tlb.FromNanoTONU(uint64(g.cost)).String(),
		"history":       g.history,
		"players":       g.GetPlayers(),
		"state":         g.GetState(),
		"max_players":   g.GetMaxPlayers(),
		"creator":       g.GetCreator(),
		"game_type":     g.GetGameType(),
	}
}

func (g *gameRecord) GetCost() int64 {
	return g.cost
}

func (g *gameRecord) GetCreator() string {
	return g.creator
}

func (g *gameDb) gameFromDao(ctx context.Context, dao *game) (*gameRecord, error) {
	cost := tlb.FromNanoTONU(dao.Cost)
	pl := make([]string, len(dao.Players))
	for i, player := range dao.Players {
		pl[i] = player.ID.String()
	}

	hrs := make([]*historyRecord, len(dao.History))
	for i, record := range dao.History {
		hrs[i] = &historyRecord{
			Timestamp: record.Timestamp,
			HType:     record.Type,
			Msg:       record.Message,
			MD:        record.MD,
		}
	}

	gr := &gameRecord{
		id:           dao.ID.String(),
		cost:         cost.Nano().Int64(),
		creationTime: dao.CreatedAt,
		players:      pl,
		state:        dao.State,
		maxPlayers:   dao.MaxPlayers,
		duration:     dao.Duration,
		history:      hrs,
		creator:      dao.Creator,
		gameType:     dao.Type,
	}

	return gr, nil
}
