package server

import (
	"context"
	"log"
	"strings"

	"github.com/PxyUp/ton_games_example/pkg/config"
	"github.com/PxyUp/ton_games_example/pkg/database"
	logger2 "github.com/PxyUp/ton_games_example/pkg/logger"
	"github.com/PxyUp/ton_games_example/pkg/server"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/wallet"
)

func initTonClient(cfgUrl string) ton.APIClientWrapped {
	client := liteclient.NewConnectionPool()
	err := client.AddConnectionsFromConfigUrl(context.Background(), cfgUrl)
	if err != nil {
		log.Fatal(err)
	}

	return ton.NewAPIClient(client).WithRetry()
}

func NewServer(ctx context.Context, payment database.PaymentDB, logger logger2.Logger) (server.Server, string, *tlb.Account) {
	client := initTonClient(config.Config.TonChainAddress)

	seed := strings.Split(config.Config.Seed, " ")

	w, err := wallet.FromSeed(client, seed, wallet.V4R2)
	if err != nil {
		log.Fatal(err)
	}

	logger.Infow("wallet init", "address", w.WalletAddress().String())

	master, err := client.CurrentMasterchainInfo(ctx) // we fetch block just to trigger chain proof check
	if err != nil {
		log.Fatal(err)
	}

	acc, err := client.GetAccount(ctx, master, w.WalletAddress())
	if err != nil {
		log.Fatal(err)
	}

	return server.New(client, payment, w, logger), w.WalletAddress().String(), acc
}
