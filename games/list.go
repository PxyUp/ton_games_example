package games

import (
	"fmt"
	"time"
)

const (
	MoreLess GameType = iota
	RockPaperScissors
)

var (
	validTypes = []string{fmt.Sprintf("%d", MoreLess)}
)

func ValidGameType(s string) bool {
	for _, f := range validTypes {
		if f == s {
			return true
		}
	}
	return false
}

type MoreLessConfig struct {
	Cost            float64       `json:"cost"`
	NumberOfPlayers uint8         `json:"number_of_players"`
	Duration        time.Duration `json:"duration"`
	WaitAll         bool          `json:"wait_all"`
	MaxRandom       uint32        `json:"max_random"`
}

type RockPaperConfig struct {
	Cost            float64       `json:"cost"`
	NumberOfPlayers uint8         `json:"number_of_players"`
	Duration        time.Duration `json:"duration"`
}
