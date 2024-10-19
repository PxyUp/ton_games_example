package database

import (
	"context"
	"time"

	"github.com/PxyUp/ton_games_example/games"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type PaymentState int8

const (
	Pending PaymentState = iota
	Finished
	Error
)

type TxType int8

const (
	In TxType = iota
	Out
)

func (g *gameDb) createAccountsTable(ctx context.Context) error {
	_, err := g.db.NewCreateTable().
		IfNotExists().
		Model((*account)(nil)).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*account)(nil)).
		Index("idx_accounts_address").
		Column("address").
		Unique().
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

type account struct {
	bun.BaseModel `bun:"table:accounts"`

	ID        uuid.UUID `bun:"type:uuid,pk"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	Games []*game `bun:"m2m:account_games,join:Account=Game"`

	Lock    []*lock `bun:"rel:has-many,join:id=account_id"`
	Winners []*win  `bun:"rel:has-many,join:id=account_id"`

	Address string `bun:"address,notnull"`
}

func (g *gameDb) createGamesTable(ctx context.Context) error {
	_, err := g.db.NewCreateTable().
		IfNotExists().
		Model((*game)(nil)).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*game)(nil)).
		Index("idx_games_creator").
		Column("creator").
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*game)(nil)).
		Index("idx_games_state").
		Column("state").
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*game)(nil)).
		Index("idx_games_type").
		Column("type").
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*game)(nil)).
		Index("state_type").
		Column("type", "state").
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

type game struct {
	bun.BaseModel `bun:"table:games"`

	ID        uuid.UUID `bun:"type:uuid,pk"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	Players []*account `bun:"m2m:account_games,join:Game=Account"`

	Lock    []*lock    `bun:"rel:has-many,join:id=game_id"`
	Winners []*win     `bun:"rel:has-many,join:id=game_id"`
	History []*history `bun:"rel:has-many,join:id=game_id"`

	Creator    string          `bun:"type:uuid,notnull"`
	Cost       uint64          `bun:"cost,notnull"`
	MaxPlayers uint8           `bun:"max_players,notnull"`
	Duration   time.Duration   `bun:"duration,notnull"`
	Type       games.GameType  `bun:"type,notnull"`
	State      games.GameState `bun:"state,notnull"`
}

func (g *gameDb) createHistoryTable(ctx context.Context) error {
	_, err := g.db.NewCreateTable().
		IfNotExists().
		Model((*history)(nil)).
		WithForeignKeys().
		ForeignKey(`("game_id") REFERENCES "games" ("id") ON DELETE CASCADE`).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*history)(nil)).
		Index("idx_histories_game_id").
		Column("game_id").
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

type history struct {
	bun.BaseModel `bun:"table:histories"`

	ID int64 `bun:"id,pk,autoincrement"`

	GameID    uuid.UUID           `bun:"type:uuid,notnull"`
	Timestamp time.Time           `bun:"timestamp,notnull"`
	Message   string              `bun:"message,notnull"`
	Type      games.GameEventType `bun:"type,notnull"`
	MD        games.GameMD        `bun:"type:jsonb"`
}

func (g *gameDb) createLockTable(ctx context.Context) error {
	_, err := g.db.NewCreateTable().
		IfNotExists().
		Model((*lock)(nil)).
		WithForeignKeys().
		ForeignKey(`("game_id") REFERENCES "games" ("id") ON DELETE CASCADE`).
		ForeignKey(`("account_id") REFERENCES "accounts" ("id") ON DELETE CASCADE`).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*lock)(nil)).
		Index("game_account").
		Column("game_id", "account_id").
		Unique().
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*lock)(nil)).
		Index("idx_locks_account_id").
		Column("account_id").
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*lock)(nil)).
		Index("idx_locks_game_id").
		Column("game_id").
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

type lock struct {
	bun.BaseModel `bun:"table:locks"`

	ID int64 `bun:"id,pk,autoincrement"`

	GameID    uuid.UUID `bun:"type:uuid,notnull"`
	AccountID uuid.UUID `bun:"type:uuid,notnull"`
	Amount    uint64    `bun:"amount,notnull"`
}

func (g *gameDb) createTransactionsTable(ctx context.Context) error {
	_, err := g.db.NewCreateTable().
		IfNotExists().
		Model((*transaction)(nil)).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*transaction)(nil)).
		Index("address").
		Column("address").
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*transaction)(nil)).
		Index("address_state").
		Column("address", "state").
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*transaction)(nil)).
		Index("address_state_type").
		Column("address", "state", "type").
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*transaction)(nil)).
		Index("state_and_type").
		Column("state", "type").
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

type transaction struct {
	bun.BaseModel `bun:"table:transactions"`

	ID        []byte    `bun:"id,pk"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	Address string       `bun:"address,notnull"`
	Type    TxType       `bun:"type,notnull"`
	State   PaymentState `bun:"state,notnull"`

	Amount         int64 `bun:"amount,notnull"`
	OriginalAmount int64 `bun:"original_amount,notnull"`
}

func (g *gameDb) createSettingsTable(ctx context.Context) error {
	_, err := g.db.NewCreateTable().
		IfNotExists().
		Model((*setting)(nil)).
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

type setting struct {
	bun.BaseModel `bun:"table:settings"`

	ID        int64     `bun:"id,pk"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	LastTx uint64 `bun:"last_tx,notnull"`
}

func (g *gameDb) createAccountGame(ctx context.Context) error {
	_, err := g.db.NewCreateTable().
		IfNotExists().
		Model((*accountGame)(nil)).
		WithForeignKeys().
		ForeignKey(`("game_id") REFERENCES "games" ("id") ON DELETE CASCADE`).
		ForeignKey(`("account_id") REFERENCES "accounts" ("id") ON DELETE CASCADE`).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*accountGame)(nil)).
		Index("idx_account_games_account_id").
		Column("account_id").
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*accountGame)(nil)).
		Index("idx_account_games_game_id").
		Column("game_id").
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

type accountGame struct {
	AccountID uuid.UUID `bun:"type:uuid,pk"`
	Account   *account  `bun:"rel:belongs-to,join:account_id=id"`
	GameID    uuid.UUID `bun:"type:uuid,pk"`
	Game      *game     `bun:"rel:belongs-to,join:game_id=id"`
}

func (g *gameDb) createWinGame(ctx context.Context) error {
	_, err := g.db.NewCreateTable().
		IfNotExists().
		Model((*win)(nil)).
		WithForeignKeys().
		ForeignKey(`("game_id") REFERENCES "games" ("id")`).
		ForeignKey(`("account_id") REFERENCES "accounts" ("id") ON DELETE CASCADE`).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*win)(nil)).
		Index("account_id_game_id").
		Column("account_id", "game_id").
		Unique().
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*win)(nil)).
		Index("idx_wins_account_id").
		Column("account_id").
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*win)(nil)).
		Index("idx_wins_game_id").
		Column("game_id").
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

type win struct {
	bun.BaseModel `bun:"table:wins"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	GameID    uuid.UUID `bun:"type:uuid,notnull"`
	AccountID uuid.UUID `bun:"type:uuid,notnull"`

	Amount int64
}

func (g *gameDb) createBonusesGame(ctx context.Context) error {
	_, err := g.db.NewCreateTable().
		IfNotExists().
		Model((*bonus)(nil)).
		WithForeignKeys().
		ForeignKey(`("account_id") REFERENCES "accounts" ("id") ON DELETE CASCADE`).
		Exec(ctx)
	if err != nil {
		return err
	}

	_, err = g.db.NewCreateIndex().
		IfNotExists().
		Model((*bonus)(nil)).
		Index("idx_bonuses_account_id").
		Column("account_id").
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

type bonus struct {
	bun.BaseModel `bun:"table:bonuses"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	AccountID uuid.UUID `bun:"type:uuid,notnull"`

	Amount int64 `bun:"amount,notnull"`
}
