package router

import (
	"encoding/json"

	"github.com/PxyUp/ton_games_example/games"
	"github.com/PxyUp/ton_games_example/games/rock_paper_scissors"
)

type MoreLessApiConfig struct {
	Cost            float64 `json:"cost"`
	NumberOfPlayers uint8   `json:"number_of_players"`
	Duration        int     `json:"duration"`
	MaxRandom       uint32  `json:"max_random"`
}

type RockPaperScissorsJoinCfg struct {
	Choice rock_paper_scissors.Choice `json:"choice"`
}

type RockPaperScissorsConfig struct {
	Choice          rock_paper_scissors.Choice `json:"choice"`
	Cost            float64                    `json:"cost"`
	NumberOfPlayers uint8                      `json:"number_of_players"`
	Duration        int                        `json:"duration"`
}

type choiceEvent struct {
	userId  string
	payload []byte
}

func (c *choiceEvent) GetPlayerId() string {
	return c.userId
}

func (c *choiceEvent) GetRawData() json.RawMessage {
	return c.payload
}

func newChoiceEvent(choice rock_paper_scissors.Choice, userId string) (games.PlayerEvent, error) {
	payload, err := json.Marshal(&rock_paper_scissors.PlayerChoiceEvent{
		Choice: choice,
	})
	if err != nil {
		return nil, err
	}

	return &choiceEvent{
		userId:  userId,
		payload: payload,
	}, nil
}
