package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/PxyUp/ton_games_example/cmd/app/router"
	"github.com/PxyUp/ton_games_example/cmd/app/server"
	"github.com/PxyUp/ton_games_example/pkg/config"
	"github.com/PxyUp/ton_games_example/pkg/database"
	logger2 "github.com/PxyUp/ton_games_example/pkg/logger"
	"github.com/PxyUp/ton_games_example/pkg/runtime"
	"github.com/PxyUp/ton_games_example/pkg/telegram"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bunotel"
	"github.com/uptrace/uptrace-go/uptrace"
)

func main() {
	mainCtx := context.Background()
	logger := logger2.NewLogger(config.Config.LoggerLevel)

	setupMonitoring()

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(config.Config.DBDsn)))

	bunDb := bun.NewDB(sqldb, pgdialect.New())
	defer func() {
		bunDb.Close()
	}()

	if !config.Config.Local {
		bunDb.AddQueryHook(bunotel.NewQueryHook(bunotel.WithDBName("postgres")))

		defer func() {
			_ = uptrace.Shutdown(mainCtx)
		}()
	}

	gameEngine, err := database.New(mainCtx, bunDb, logger.With("component", "database"), config.Config.SettingsID)
	if err != nil {
		log.Fatal(err)
	}

	rt := runtime.New(mainCtx, gameEngine, logger.With("component", "runtime"))

	bot := telegram.New(mainCtx, logger.With("component", "bot"))

	srv, address, acc := server.NewServer(mainCtx, gameEngine, logger.With("component", "ton_server"))
	go func() {
		log.Fatal(srv.Listen(mainCtx, acc))
	}()

	log.Fatal(router.New(gameEngine, rt, srv, address, bot.WebHookHandler, logger.With("component", "router")).Start(fmt.Sprintf(":%v", config.Config.Port)))
}

func setupMonitoring() {
	if config.Config.Local {
		return
	}

	uptrace.ConfigureOpentelemetry(
		// copy your project DSN here or use UPTRACE_DSN env var
		uptrace.WithDSN(config.Config.UptraceDSN),
		uptrace.WithServiceName("global"),
		uptrace.WithServiceVersion("v1.0.0"),
		uptrace.WithDeploymentEnvironment("production"),
	)
}
