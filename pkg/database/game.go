package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PxyUp/ton_games_example/games"
	"github.com/PxyUp/ton_games_example/pkg/config"
	"github.com/PxyUp/ton_games_example/pkg/logger"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/xssnick/tonutils-go/tlb"
	"golang.org/x/sync/errgroup"
)

var (
	_ GameDB = &gameDb{}
)

var (
	ErrMinimumWithdrawal        = fmt.Errorf("minimum withdrawal: %s", tlb.FromNanoTONU(uint64(config.MIN_WITHDRAW_AMOUNT)).String())
	ErrMaxGamesInProgress       = fmt.Errorf("max games in progress: %d", config.Config.MaxGamesInProgress)
	ErrMaxPlayersInGame         = fmt.Errorf("max players in game")
	ErrMaxPlayerGamesInProgress = fmt.Errorf("max games per player progress: %d", config.Config.MaxPlayerGamesInProgress)
	ErrSmallBalance             = errors.New("small balance")
	ErrCreatorCantLeftGame      = errors.New("creator cant left game")
	ErrCreatorCantJoinGame      = errors.New("creator already part of the game")
)

type GameDB interface {
	CreateGame(ctx context.Context, game games.Game) (GameRecord, error)
	GetGameById(ctx context.Context, gameId string, pairs ...*preloadPair) (GameRecord, error)
	JoinGame(ctx context.Context, game games.Game, playerID string, cb func() error) (GameRecord, error)
	LeftGame(ctx context.Context, game games.Game, playerID string, cb func() error) (GameRecord, error)
	AppendEvent(ctx context.Context, gameInstant games.Game, gevent games.GameEvent) (GameRecord, error)
	ChangeGameState(ctx context.Context, game games.Game, state games.GameState) (GameRecord, error)
	UnlockAllPlayer(ctx context.Context, game games.Game) (GameRecord, error)
	StoreWinners(ctx context.Context, game games.Game, winners []games.Player) (GameRecord, error)
	GetActiveGames(ctx context.Context, gameType games.GameType) ([]GameRecord, error)
}

type gameDb struct {
	settingsID uint
	logger     logger.Logger
	db         *bun.DB
}

func (g *gameDb) unlockAllInProgressGames(ctx context.Context) error {
	return g.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		gList := []*game{}

		errGames := g.db.NewSelect().Model(&gList).Column("id").Where("state IN (?)", bun.In([]games.GameState{games.GameInProgress, games.GameCreated})).Scan(ctx)
		if errGames != nil {
			g.logger.Errorf("cant select games in states: %v", []games.GameState{games.GameInProgress, games.GameCreated})
			return errGames
		}

		var eg errgroup.Group

		for lI := range gList {
			index := lI
			eg.Go(func() error {
				gameId := gList[index].ID
				_, errUpdate := g.db.NewUpdate().Model((*game)(nil)).Set("state = ?", games.GameFinished).Where("id = ?", gameId).Exec(ctx)
				if errUpdate != nil {
					return errUpdate
				}

				_, errDelete := g.db.NewDelete().Model((*lock)(nil)).Where("game_id = ?", gameId).Exec(ctx)
				if errDelete != nil {
					return errDelete
				}

				return nil
			})
		}

		errAllDeleted := eg.Wait()
		if errAllDeleted != nil {
			g.logger.Errorw("cant delete all lock", "error", errAllDeleted.Error())
			return errAllDeleted
		}

		return nil
	})
}

func (g *gameDb) GetActiveGames(ctx context.Context, gameType games.GameType) ([]GameRecord, error) {
	gList := []*game{}

	errList := g.db.NewSelect().Model(&gList).Where("state IN (?)", bun.In([]games.GameState{games.GameInProgress, games.GameCreated})).Where("type = ?", gameType).Order("created_at desc").Relation("Players", func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Column("id")
	}).Scan(ctx)
	if errList != nil {
		return nil, g.hideError(errList)
	}

	gr := make([]GameRecord, len(gList))

	var eg errgroup.Group

	for i := range gList {
		lI := i
		eg.Go(func() error {
			gRecord, errDao := g.gameFromDao(ctx, gList[lI])
			if errDao != nil {
				return errDao
			}

			gr[lI] = gRecord
			return nil
		})
	}

	errGr := eg.Wait()
	if errGr != nil {
		return nil, g.hideError(errGr)
	}

	return gr, nil
}

func (g *gameDb) StoreWinners(ctx context.Context, gameInstant games.Game, winnersList []games.Player) (GameRecord, error) {
	gameIdUuid, err := uuid.Parse(gameInstant.GetID())
	if err != nil {
		return nil, g.hideError(err)
	}
	err = g.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, errDeleteLock := tx.NewDelete().Model((*lock)(nil)).Where("game_id = ?", gameIdUuid).Exec(ctx)
		if errDeleteLock != nil {
			return errDeleteLock
		}

		gameDao := &game{}
		errGamePlayer := tx.NewSelect().Model(gameDao).Where("id = ?", gameIdUuid).Relation("Players", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Column("id")
		}).Scan(ctx)
		if errGamePlayer != nil {
			return errGamePlayer
		}

		var losers []*win
		var winners []*win

		gamePrice := uint64(time.Duration(gameInstant.GetCost() * float64(time.Second)))
		bank := gamePrice * uint64(len(gameDao.Players))

		winnerGet := bank/uint64(len(winnersList)) - gamePrice

		timeNow := time.Now()

		for _, p := range gameDao.Players {
			exists := false
			for _, winner := range winnersList {
				if p.ID.String() == winner.GetId() {
					exists = true
					break
				}
			}

			if exists {
				winners = append(winners, &win{
					GameID:    gameIdUuid,
					AccountID: p.ID,
					Amount:    int64(winnerGet),
					CreatedAt: timeNow,
					UpdatedAt: timeNow,
				})
			} else {
				losers = append(losers, &win{
					GameID:    gameIdUuid,
					AccountID: p.ID,
					Amount:    -int64(gamePrice),
					CreatedAt: timeNow,
					UpdatedAt: timeNow,
				})
			}
		}

		all := append(winners, losers...)

		_, errInsert := tx.NewInsert().Model(&all).Exec(ctx)
		if errInsert != nil {
			return errInsert
		}

		return nil
	})
	if err != nil {
		return nil, g.hideError(err)
	}

	return g.GetGameById(ctx, gameInstant.GetID())
}

func (g *gameDb) UnlockAllPlayer(ctx context.Context, gameInstant games.Game) (GameRecord, error) {
	gameIdUuid, err := uuid.Parse(gameInstant.GetID())
	if err != nil {
		return nil, g.hideError(err)
	}

	_, errDelete := g.db.NewDelete().Model((*lock)(nil)).Where("game_id = ?", gameIdUuid).Exec(ctx)
	if errDelete != nil {
		return nil, g.hideError(errDelete)
	}

	return g.GetGameById(ctx, gameInstant.GetID())
}

func (g *gameDb) ChangeGameState(ctx context.Context, gameInstant games.Game, state games.GameState) (GameRecord, error) {
	gameIdUuid, err := uuid.Parse(gameInstant.GetID())
	if err != nil {
		return nil, g.hideError(err)
	}

	_, errUpdate := g.db.NewUpdate().Model((*game)(nil)).Set("state = ?", state).Set("updated_at = ?", time.Now()).Where("id = ?", gameIdUuid).Exec(ctx)
	if errUpdate != nil {
		return nil, g.hideError(errUpdate)
	}

	return g.GetGameById(ctx, gameInstant.GetID())
}

func (g *gameDb) AppendEvent(ctx context.Context, gameInstant games.Game, gevent games.GameEvent) (GameRecord, error) {
	gameIdUuid, err := uuid.Parse(gameInstant.GetID())
	if err != nil {
		return nil, g.hideError(err)
	}

	_, errAppend := g.db.NewInsert().Model(&history{
		GameID:    gameIdUuid,
		Timestamp: gevent.GetTimeStamp(),
		Message:   gevent.Msg(),
		Type:      gevent.GetEventType(),
		MD:        gevent.GetMD(),
	}).Exec(ctx)
	if errAppend != nil {
		return nil, g.hideError(errAppend)
	}

	return g.GetGameById(ctx, gameInstant.GetID(), NewPreload("History"))
}

func (g *gameDb) LeftGame(ctx context.Context, gameInstant games.Game, playerID string, cb func() error) (GameRecord, error) {
	if playerID == gameInstant.GetCreator() {
		return nil, ErrCreatorCantLeftGame
	}

	gameIdUuid, err := uuid.Parse(gameInstant.GetID())
	if err != nil {
		return nil, g.hideError(err)
	}

	playerIDUuid, err := uuid.Parse(playerID)
	if err != nil {
		return nil, g.hideError(err)
	}

	err = g.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, errLock := tx.NewDelete().Model((*lock)(nil)).Where("game_id = ?", gameIdUuid).Where("account_id = ?", playerIDUuid).Exec(ctx)
		if errLock != nil {
			return errLock
		}

		_, errPlayerGame := tx.NewDelete().Model((*accountGame)(nil)).Where("game_id = ?", gameIdUuid).Where("account_id = ?", playerIDUuid).Exec(ctx)
		if errPlayerGame != nil {
			return errPlayerGame
		}

		errCb := cb()
		if errCb != nil {
			return errCb
		}

		return nil
	})
	if err != nil {
		return nil, g.hideError(err)
	}

	return g.GetGameById(ctx, gameInstant.GetID())
}

func (g *gameDb) JoinGame(ctx context.Context, gameInstant games.Game, playerID string, cb func() error) (GameRecord, error) {
	if playerID == gameInstant.GetCreator() {
		return nil, ErrCreatorCantJoinGame
	}

	valid, cost, err := g.canPlayerJoinGame(ctx, gameInstant, playerID)
	if err != nil {
		return nil, g.hideError(err)
	}

	if !valid {
		return nil, ErrSmallBalance
	}

	gameIdUuid, err := uuid.Parse(gameInstant.GetID())
	if err != nil {
		return nil, g.hideError(err)
	}

	playerIDUuid, err := uuid.Parse(playerID)
	if err != nil {
		return nil, g.hideError(err)
	}

	err = g.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		count, errCount := tx.NewSelect().Model((*accountGame)(nil)).Where("game_id = ?", gameIdUuid).Count(ctx)
		if errCount != nil {
			return errCount
		}

		if count+1 > int(gameInstant.GetMaxPlayers()) {
			return ErrMaxPlayersInGame
		}

		_, errLockAppend := tx.NewInsert().Model(&lock{
			GameID:    gameIdUuid,
			AccountID: playerIDUuid,
			Amount:    cost,
		}).Exec(ctx)
		if errLockAppend != nil {
			return errLockAppend
		}

		_, errGameAccount := tx.NewInsert().Model(&accountGame{
			GameID:    gameIdUuid,
			AccountID: playerIDUuid,
		}).Exec(ctx)
		if errGameAccount != nil {
			return errGameAccount
		}

		errCb := cb()
		if errCb != nil {
			return errCb
		}

		return nil
	})
	if err != nil {
		return nil, g.hideError(err)
	}

	return g.GetGameById(ctx, gameInstant.GetID())
}

func (g *gameDb) GetGameById(ctx context.Context, gameId string, pairs ...*preloadPair) (GameRecord, error) {
	gameIdUuid, err := uuid.Parse(gameId)
	if err != nil {
		return nil, g.hideError(err)
	}

	gr := &game{}

	errScan := buildPreload(g.db.NewSelect().Model(gr).Where("id = ?", gameIdUuid), pairs...).Scan(ctx)
	if errScan != nil {
		return nil, g.hideError(errScan)
	}

	return g.gameFromDao(ctx, gr)
}

func (g *gameDb) CreateGame(ctx context.Context, gameInstant games.Game) (GameRecord, error) {
	valid, cost, err := g.canPlayerJoinGame(ctx, gameInstant, gameInstant.GetCreator())
	if err != nil {
		return nil, g.hideError(err)
	}

	if !valid {
		return nil, ErrSmallBalance
	}

	gameIdUuid, err := uuid.Parse(gameInstant.GetID())
	if err != nil {
		return nil, g.hideError(err)
	}

	creatorIDUuid, err := uuid.Parse(gameInstant.GetCreator())
	if err != nil {
		return nil, g.hideError(err)
	}

	var errGr errgroup.Group

	errGr.Go(func() error {
		listGames := []*game{}
		count, errCount := g.db.NewSelect().Model(&listGames).Where("state IN (?)", bun.In([]games.GameState{games.GameInProgress, games.GameCreated})).Count(ctx)
		if errCount != nil {
			return errCount
		}

		if count >= int(config.Config.MaxGamesInProgress) {
			return ErrMaxGamesInProgress
		}

		return nil
	})

	errGr.Go(func() error {
		listGames := []*game{}
		count, errCount := g.db.NewSelect().Model(&listGames).Where("state IN (?)", bun.In([]games.GameState{games.GameInProgress, games.GameCreated})).Where("creator = ?", creatorIDUuid).Count(ctx)
		if errCount != nil {
			return errCount
		}

		if count >= int(config.Config.MaxPlayerGamesInProgress) {
			return ErrMaxPlayerGamesInProgress
		}

		return nil
	})

	errGroupErr := errGr.Wait()
	if errGroupErr != nil {
		return nil, g.hideError(errGroupErr)
	}

	errTx := g.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		timeNow := time.Now()
		gr := &game{
			ID:         gameIdUuid,
			CreatedAt:  timeNow,
			UpdatedAt:  timeNow,
			Creator:    gameInstant.GetCreator(),
			Cost:       cost,
			MaxPlayers: gameInstant.GetMaxPlayers(),
			Duration:   gameInstant.GetDuration(),
			Type:       gameInstant.GameType(),
			State:      games.GameCreated,
		}

		_, errCreated := tx.NewInsert().Model(gr).Exec(ctx)
		if errCreated != nil {
			return errCreated
		}

		_, errLockAppend := tx.NewInsert().Model(&lock{
			GameID:    gameIdUuid,
			AccountID: creatorIDUuid,
			Amount:    cost,
		}).Exec(ctx)
		if errLockAppend != nil {
			return errLockAppend
		}

		_, errGameAccount := tx.NewInsert().Model(&accountGame{
			GameID:    gameIdUuid,
			AccountID: creatorIDUuid,
		}).Exec(ctx)
		if errGameAccount != nil {
			return errGameAccount
		}

		return nil
	})
	if errTx != nil {
		return nil, g.hideError(errTx)
	}

	return g.GetGameById(ctx, gameInstant.GetID())
}

func (g *gameDb) canPlayerJoinGame(ctx context.Context, gameInstant games.Game, playerId string) (bool, uint64, error) {
	playerIDUuid, err := uuid.Parse(playerId)
	if err != nil {
		return false, 0, g.hideError(err)
	}

	balance, err := g.GetBalanceByPlayerID(ctx, playerIDUuid)
	if err != nil {
		return false, 0, g.hideError(err)
	}

	floatCost := uint64(time.Duration(gameInstant.GetCost() * float64(time.Second)))

	if balance.Available() < floatCost {
		return false, 0, nil
	}

	return true, floatCost, nil
}
