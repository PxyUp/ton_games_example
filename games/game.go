package games

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalidAction   = errors.New("invalid action")
	ErrMaxPlayer       = errors.New("reach max players")
	ErrGameFinished    = errors.New("game already finished")
	ErrCreatorCantLeft = errors.New("creator cant left from the game")
)

type Player interface {
	GetId() string
}

type BasePlayer struct {
	Id string
}

func (p *BasePlayer) GetId() string {
	return p.Id
}

type GameMD map[string]interface{}

type Game interface {
	GetID() string
	GetCost() float64
	GameType() GameType
	GetDuration() time.Duration
	AddPlayer(Player) error
	AddPlayerWithAction(Player, PlayerEvent) error
	RemovePlayer(Player) error
	Start(ctx context.Context) error
	Abort() error
	GetWinners() ([]Player, GameMD, error)
	Updates() <-chan GameEvent
	SendUserEvent(PlayerEvent) error
	GetCreator() string
	GetMaxPlayers() uint8
}

type GameEventType int8

type GameType int8

const (
	Start GameEventType = iota
	Update
	PlayerJoin
	PlayerLeft
	Abort
	Winners
	NoWinners
	Finished
	Error
)

type GameState int8

const (
	GameCreated GameState = iota
	GameInProgress
	GameFinished
	GameError
)

type PlayerEvent interface {
	GetPlayerId() string
	GetRawData() json.RawMessage
}

type GameEvent interface {
	GameId() string
	GetEventType() GameEventType
	Msg() string
	GetTimeStamp() time.Time
	IsPublic() bool
	Players() []Player
	GetMD() GameMD
}
