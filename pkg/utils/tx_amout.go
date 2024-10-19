package utils

import (
	"math/big"

	"github.com/PxyUp/ton_games_example/pkg/config"
)

func FromOutToFull(value *big.Int) int64 {
	return int64(config.COMMISSION*float64(value.Int64())) + config.TX_FEE
}

func FromFullToOut(value *big.Int) int64 {
	return int64((1/config.COMMISSION)*float64(value.Int64())) - config.TX_FEE
}
