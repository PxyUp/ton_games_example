package games

import (
	"time"
)

var (
	_ GameEvent = &gameEvent{}
)

type gameEvent struct {
	gameId    string
	eventType GameEventType
	timestamp time.Time
	msg       string
	public    bool
	players   []Player
	md        GameMD
}

func (e *gameEvent) GetMD() GameMD {
	return e.md
}

func (e *gameEvent) GameId() string {
	return e.gameId
}

func (e *gameEvent) Players() []Player {
	return e.players
}

func (e *gameEvent) GetEventType() GameEventType {
	return e.eventType
}

func (e *gameEvent) Msg() string {
	return e.msg
}

func (e *gameEvent) GetTimeStamp() time.Time {
	return e.timestamp
}

func (e *gameEvent) IsPublic() bool {
	return e.public
}

func NewGameEvent(gameId string, eventType GameEventType, msg string, public bool, players []Player, md GameMD) *gameEvent {
	return &gameEvent{
		gameId:    gameId,
		eventType: eventType,
		timestamp: time.Now(),
		msg:       msg,
		public:    public,
		players:   players,
		md:        md,
	}
}
