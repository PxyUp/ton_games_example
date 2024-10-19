package runtime

import (
	"context"
	"errors"
	"sync"

	"github.com/PxyUp/ton_games_example/games"
	"github.com/PxyUp/ton_games_example/pkg/database"
	"github.com/PxyUp/ton_games_example/pkg/logger"
)

type runtime struct {
	ctx context.Context

	mutex sync.Mutex
	kv    map[string]games.Game

	log   logger.Logger
	store database.DB
}

func (r *runtime) JoinGameWithAction(ctx context.Context, game games.Game, playerID string, action games.PlayerEvent) (games.Game, error) {
	_, err := r.store.JoinGame(ctx, game, playerID, func() error {
		return game.AddPlayerWithAction(&games.BasePlayer{
			Id: playerID,
		}, action)
	})
	if err != nil {
		return nil, err
	}

	return game, nil
}

func (r *runtime) LeftGame(ctx context.Context, game games.Game, playerID string) (games.Game, error) {
	_, err := r.store.LeftGame(ctx, game, playerID, func() error {
		return game.RemovePlayer(&games.BasePlayer{
			Id: playerID,
		})
	})
	if err != nil {
		return nil, err
	}

	return game, nil
}

var (
	ErrInvalidGameID = errors.New("invalid game id")
)

func (r *runtime) GetGame(ctx context.Context, id string) (games.Game, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	val, exists := r.kv[id]
	if !exists {
		return nil, ErrInvalidGameID
	}

	return val, nil
}

func (r *runtime) SendUserEvent(ctx context.Context, game games.Game, event games.PlayerEvent) error {
	return game.SendUserEvent(event)
}

func (r *runtime) JoinGame(ctx context.Context, game games.Game, playerID string) (games.Game, error) {
	_, err := r.store.JoinGame(ctx, game, playerID, func() error {
		return game.AddPlayer(&games.BasePlayer{
			Id: playerID,
		})
	})
	if err != nil {
		return nil, err
	}

	return game, nil
}

func (r *runtime) SubscribeOnGame(game games.Game) error {
	r.mutex.Lock()
	gameId := game.GetID()
	_, exists := r.kv[gameId]
	if exists {
		r.mutex.Unlock()
		return nil
	}

	err := game.Start(r.ctx)
	if err != nil {
		r.mutex.Unlock()
		return err
	}

	gameCreated := make(chan struct{})

	go func() {
		defer func() {
			r.mutex.Lock()
			delete(r.kv, gameId)
			r.mutex.Unlock()
		}()
		<-gameCreated
		for event := range game.Updates() {
			if event.IsPublic() {
				_, errAppend := r.store.AppendEvent(r.ctx, game, event)
				if errAppend != nil {
					r.log.Errorw("cant append event to the game", "error", errAppend.Error())
				}
			}
			switch event.GetEventType() {
			case games.NoWinners:
				_, errUnlockPlayer := r.store.UnlockAllPlayer(r.ctx, game)
				if errUnlockPlayer != nil {
					r.log.Errorw("cant unlock all players", "error", errUnlockPlayer.Error())
				}
			case games.Finished:
				_, errState := r.store.ChangeGameState(r.ctx, game, games.GameFinished)
				if errState != nil {
					r.log.Errorw("cant update game state", "error", errState.Error())
				}
			case games.Start:
				_, errState := r.store.ChangeGameState(r.ctx, game, games.GameInProgress)
				if errState != nil {
					r.log.Errorw("cant update game state", "error", errState.Error())
				}
			case games.Abort:
				_, errState := r.store.ChangeGameState(r.ctx, game, games.GameError)
				if errState != nil {
					r.log.Errorw("cant update game state", "error", errState.Error())
				}
				_, errUnlockPlayer := r.store.UnlockAllPlayer(r.ctx, game)
				if errUnlockPlayer != nil {
					r.log.Errorw("cant unlock all players", "error", errUnlockPlayer.Error())
				}
			case games.Winners:
				_, errWinners := r.store.StoreWinners(r.ctx, game, event.Players())
				if errWinners != nil {
					r.log.Errorw("cant store winners", "error", errWinners.Error())
				}
			case games.PlayerJoin:
				continue
			case games.PlayerLeft:
				continue
			case games.Error:
				_, errState := r.store.ChangeGameState(r.ctx, game, games.GameError)
				if errState != nil {
					r.log.Errorw("cant update game state", "error", errState.Error())
				}
				_, errUnlockPlayer := r.store.UnlockAllPlayer(r.ctx, game)
				if errUnlockPlayer != nil {
					r.log.Errorw("cant unlock all players", "error", errUnlockPlayer.Error())
				}
			default:
				continue
			}
		}
	}()

	_, err = r.store.CreateGame(r.ctx, game)
	if err != nil {
		_ = game.Abort()
		r.mutex.Unlock()
		return err
	}

	close(gameCreated)

	r.kv[gameId] = game
	r.mutex.Unlock()

	return nil
}

func (r *runtime) ListOfGames(ctx context.Context) ([]games.Game, error) {
	r.mutex.Lock()
	list := make([]games.Game, len(r.kv))
	i := 0
	for _, k := range r.kv {
		list[i] = k
		i += 1
	}
	r.mutex.Unlock()

	return list, nil
}

type Runtime interface {
	ListOfGames(ctx context.Context) ([]games.Game, error)
	GetGame(ctx context.Context, id string) (games.Game, error)
	JoinGame(ctx context.Context, game games.Game, playerID string) (games.Game, error)
	JoinGameWithAction(ctx context.Context, game games.Game, playerID string, action games.PlayerEvent) (games.Game, error)
	LeftGame(ctx context.Context, game games.Game, playerID string) (games.Game, error)
	SendUserEvent(ctx context.Context, game games.Game, event games.PlayerEvent) error
	SubscribeOnGame(game games.Game) error
}

func New(ctx context.Context, store database.DB, logger2 logger.Logger) Runtime {
	return &runtime{
		store: store,
		ctx:   ctx,
		log:   logger2,
		kv:    make(map[string]games.Game),
	}
}
