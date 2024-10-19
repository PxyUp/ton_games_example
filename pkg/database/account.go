package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	_ AccountDB = &gameDb{}
)

type AccountDB interface {
	GetPlayerByAddress(ctx context.Context, address string) (AccountRecord, error)
	GetPlayerById(ctx context.Context, ID uuid.UUID) (AccountRecord, error)
	CreatePlayer(ctx context.Context, address string) (AccountRecord, error)
}

func (g *gameDb) getPlayerByDao(ctx context.Context, dao *account, query string, args ...interface{}) (AccountRecord, error) {
	err := g.db.NewSelect().Model(dao).Where(query, args...).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMissingPlayer
		}
		return nil, g.hideError(err)
	}

	return g.accountFromDao(ctx, dao)
}

func (g *gameDb) GetPlayerById(ctx context.Context, ID uuid.UUID) (AccountRecord, error) {
	return g.getPlayerByDao(ctx, &account{}, "id = ?", ID)
}

func (g *gameDb) GetPlayerByAddress(ctx context.Context, address string) (AccountRecord, error) {
	return g.getPlayerByDao(ctx, &account{}, "address = ?", address)
}

func (g *gameDb) CreatePlayer(ctx context.Context, address string) (AccountRecord, error) {
	nn := time.Now()
	r := &account{
		ID:        uuid.New(),
		Address:   address,
		CreatedAt: nn,
		UpdatedAt: nn,
	}

	_, err := g.db.NewInsert().Model(r).Exec(ctx)
	if err != nil {
		return nil, err
	}

	return g.GetPlayerById(ctx, r.ID)
}
