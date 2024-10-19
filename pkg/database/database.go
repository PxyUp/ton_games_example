package database

import (
	"context"
	"errors"
	"runtime"

	"github.com/PxyUp/ton_games_example/pkg/config"
	"github.com/PxyUp/ton_games_example/pkg/logger"
	"github.com/uptrace/bun"
)

var (
	ErrMissingPlayer    = errors.New("missing player by id")
	ErrInternalDBError  = errors.New("internal error")
	ErrTxRecordNotFound = errors.New("tx record not found")
)

func (g *gameDb) createUUIDExtension(ctx context.Context) error {
	_, err := g.db.NewRaw(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`).Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

func New(ctx context.Context, db *bun.DB, logger logger.Logger, settingsID uint) (DB, error) {
	database := &gameDb{
		logger:     logger,
		settingsID: settingsID,
		db:         db,
	}

	maxOpenConns := 4 * runtime.GOMAXPROCS(0)
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)

	err := db.Ping()
	if err != nil {
		return nil, err
	}

	database.db.RegisterModel((*accountGame)(nil))

	if config.Config.WithDBSchema {
		err = database.createUUIDExtension(ctx)
		if err != nil {
			logger.Errorw("cant exec createUUIDExtension", "error", err.Error())
			return nil, err
		}

		err = database.createAccountsTable(ctx)
		if err != nil {
			logger.Errorw("cant exec createAccountsTable", "error", err.Error())
			return nil, err
		}

		err = database.createGamesTable(ctx)
		if err != nil {
			logger.Errorw("cant exec createGamesTable", "error", err.Error())
			return nil, err
		}

		err = database.createAccountGame(ctx)
		if err != nil {
			logger.Errorw("cant exec createAccountGame", "error", err.Error())
			return nil, err
		}

		err = database.createHistoryTable(ctx)
		if err != nil {
			logger.Errorw("cant exec createHistoryTable", "error", err.Error())
			return nil, err
		}

		err = database.createLockTable(ctx)
		if err != nil {
			logger.Errorw("cant exec createLockTable", "error", err.Error())
			return nil, err
		}

		err = database.createWinGame(ctx)
		if err != nil {
			logger.Errorw("cant exec createWinGame", "error", err.Error())
			return nil, err
		}

		err = database.createTransactionsTable(ctx)
		if err != nil {
			logger.Errorw("cant exec createTransactionsTable", "error", err.Error())
			return nil, err
		}

		err = database.createBonusesGame(ctx)
		if err != nil {
			logger.Errorw("cant exec createBonusesGame", "error", err.Error())
			return nil, err
		}

		err = database.createSettingsTable(ctx)
		if err != nil {
			logger.Errorw("cant exec createSettingsTable", "error", err.Error())
			return nil, err
		}

		logger.Info("schema applied")
	}

	errUnlock := database.unlockAllInProgressGames(ctx)
	if errUnlock != nil {
		logger.Errorw("cant unlock games", "error", errUnlock.Error())
		return nil, errUnlock
	}

	return database, nil
}

type preloadPair struct {
	query string
	args  []func(*bun.SelectQuery) *bun.SelectQuery
}

func NewPreload(query string, args ...func(*bun.SelectQuery) *bun.SelectQuery) *preloadPair {
	return &preloadPair{
		query: query,
		args:  args,
	}
}

func buildPreload(tx *bun.SelectQuery, preloads ...*preloadPair) *bun.SelectQuery {
	i := len(preloads) - 1
	if i < 0 {
		return tx
	}
	res := tx.Relation(preloads[i].query, preloads[i].args...)
	i -= 1
	for i >= 0 {
		res = res.Relation(preloads[i].query, preloads[i].args...)
		i -= 1
	}
	return res
}

type DB interface {
	AccountDB
	GameDB
	PaymentDB
}

func (g *gameDb) hideError(err error) error {
	if err == nil {
		return err
	}

	g.logger.Errorw("error during query execution", "error", err.Error())
	return ErrInternalDBError
}
