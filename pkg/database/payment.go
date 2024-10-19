package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PxyUp/ton_games_example/pkg/config"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/xssnick/tonutils-go/tlb"
	"golang.org/x/sync/errgroup"
)

var (
	_ BalanceRecord = &balanceRecord{}
)

type BalanceRecord interface {
	Available() uint64
	Hold() uint64
	Profit() int64
	PendingWithdrawal() uint64
	JSON() map[string]interface{}
}

type balanceRecord struct {
	available         uint64
	hold              uint64
	profit            int64
	pendingWithdrawal uint64
}

func (b *balanceRecord) JSON() map[string]interface{} {
	profit := b.Profit()
	profitStr := ""
	if profit < 0 {
		profitStr = fmt.Sprintf("- %s", tlb.FromNanoTONU(uint64(profit*-1)).String())
	} else {
		profitStr = tlb.FromNanoTONU(uint64(profit)).String()
	}

	return map[string]interface{}{
		"available":          tlb.FromNanoTONU(b.Available()).String(),
		"hold":               tlb.FromNanoTONU(b.Hold()).String(),
		"profit":             profitStr,
		"pending_withdrawal": tlb.FromNanoTONU(b.PendingWithdrawal()).String(),
	}
}

func (b *balanceRecord) Available() uint64 {
	return b.available
}

func (b *balanceRecord) Hold() uint64 {
	return b.hold
}

func (b *balanceRecord) Profit() int64 {
	return b.profit
}

func (b *balanceRecord) PendingWithdrawal() uint64 {
	return b.pendingWithdrawal
}

type PaymentDB interface {
	StoreInTx(ctx context.Context, tx TransactionRecordStore, lastTxs uint64) (TransactionRecord, error)
	StorePendingOutTx(ctx context.Context, tx TransactionRecordStore, cb func() error) (TransactionRecord, error)
	StoreOutTx(ctx context.Context, tx TransactionRecordStore, lastTxs uint64) (TransactionRecord, error)
	GetTxById(ctx context.Context, id []byte) (TransactionRecord, error)
	GetBalanceByPlayerID(ctx context.Context, ID uuid.UUID) (BalanceRecord, error)
	GetBalanceByAddress(ctx context.Context, address string) (BalanceRecord, error)
	GetLastTxID(ctx context.Context) (uint64, error)
	UpdateOutTxByID(ctx context.Context, currentID []byte, newID []byte, lastTxs uint64) (TransactionRecord, error)
	GetLastTransactionByAddress(ctx context.Context, address string, limit int) ([]TransactionRecord, error)
}

func (g *gameDb) txFromDao(ctx context.Context, dao *transaction) (TransactionRecord, error) {
	return &txRecord{
		id:        dao.ID,
		state:     dao.State,
		txType:    dao.Type,
		amount:    dao.Amount,
		createdAt: dao.CreatedAt,
		updatedAt: dao.UpdatedAt,
	}, nil
}

func (g *gameDb) GetLastTransactionByAddress(ctx context.Context, address string, limit int) ([]TransactionRecord, error) {
	arr := []*transaction{}
	errList := g.db.NewSelect().Model(&arr).Column("id", "created_at", "updated_at", "type", "state", "amount").Where("address = ?", address).Order("created_at desc").Limit(limit).Scan(ctx)
	if errList != nil {
		return nil, g.hideError(errList)
	}

	gr := make([]TransactionRecord, len(arr))

	var eg errgroup.Group

	for i := range arr {
		lI := i
		eg.Go(func() error {
			tRecord, errDao := g.txFromDao(ctx, arr[lI])
			if errDao != nil {
				return errDao
			}

			gr[lI] = tRecord
			return nil
		})
	}

	errGr := eg.Wait()
	if errGr != nil {
		return nil, g.hideError(errGr)
	}

	return gr, nil
}

func (g *gameDb) GetLastTxID(ctx context.Context) (uint64, error) {
	tt := &setting{}
	req := g.db.NewSelect().Model(tt).Where("id = ?", int64(g.settingsID))
	g.logger.Infow("request to get latest tx id", "request", req.String())
	err := req.Scan(ctx)
	if err != nil {
		return 0, g.hideError(err)
	}

	return tt.LastTx, nil
}

func (g *gameDb) UpdateOutTxByID(ctx context.Context, ID []byte, newID []byte, lastTxs uint64) (TransactionRecord, error) {
	errTx := g.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewUpdate().Model((*transaction)(nil)).Set("id = ?", newID).Set("state = ?", Finished).Set("updated_at = ?", time.Now()).Where("id = ?", ID).Where("type = ?", Out).Exec(ctx)
		if err != nil {
			return err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if count == 0 {
			return ErrTxRecordNotFound
		}

		_, errUpdate := tx.NewUpdate().Model((*setting)(nil)).Set("last_tx = ?", lastTxs).Where("id = ?", int64(g.settingsID)).Exec(ctx)
		if err != nil {
			return errUpdate
		}

		return nil
	})
	if errTx != nil {
		if errors.Is(errTx, ErrTxRecordNotFound) {
			return nil, errTx
		}
		return nil, g.hideError(errTx)
	}

	return g.GetTxById(ctx, newID)
}

func (g *gameDb) StorePendingOutTx(ctx context.Context, txx TransactionRecordStore, cb func() error) (TransactionRecord, error) {
	balance, err := g.GetBalanceByAddress(ctx, txx.GetAddress())
	if err != nil {
		return nil, g.hideError(err)
	}

	if txx.GetAmount() < config.MIN_WITHDRAW_AMOUNT {
		return nil, ErrMinimumWithdrawal
	}

	if int64(balance.Available()) < txx.GetAmount() {
		return nil, ErrSmallBalance
	}

	errTx := g.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		timeNow := time.Now()
		_, errInsert := tx.NewInsert().Model(&transaction{
			ID:             txx.GetID(),
			Address:        txx.GetAddress(),
			Type:           Out,
			State:          Pending,
			Amount:         -txx.GetAmount(),
			OriginalAmount: -txx.GetOriginalAmount(),
			CreatedAt:      timeNow,
			UpdatedAt:      timeNow,
		}).Exec(ctx)
		if errInsert != nil {
			return errInsert
		}

		errCb := cb()
		if errCb != nil {
			return errCb
		}

		return nil
	})

	if errTx != nil {
		return nil, g.hideError(errTx)
	}

	return g.GetTxById(ctx, txx.GetID())
}

func (g *gameDb) storeTx(ctx context.Context, txx *transaction, lastTxs uint64) (TransactionRecord, error) {
	errTx := g.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		count, errCount := tx.NewSelect().Model((*transaction)(nil)).Where("id = ?", txx.ID).Count(ctx)
		if errCount != nil {
			return errCount
		}
		if count == 1 {
			_, errUpdate := tx.NewUpdate().Model((*setting)(nil)).Set("last_tx = ?", lastTxs).Where("id = ?", int64(g.settingsID)).Exec(ctx)
			return errUpdate
		}

		_, errCreate := tx.NewInsert().Model(txx).Exec(ctx)
		if errCreate != nil {
			return errCreate
		}

		timeNow := time.Now()
		_, errUpdate := tx.NewInsert().On("CONFLICT (id) DO UPDATE").Set("updated_at = ?", time.Now()).Set("last_tx = ?", lastTxs).Model(&setting{
			ID:        int64(g.settingsID),
			CreatedAt: timeNow,
			UpdatedAt: timeNow,
			LastTx:    lastTxs,
		}).Exec(ctx)
		return errUpdate
	})
	if errTx != nil {
		return nil, g.hideError(errTx)
	}

	return g.GetTxById(ctx, txx.ID)
}

func (g *gameDb) StoreOutTx(ctx context.Context, tx TransactionRecordStore, lastTxs uint64) (TransactionRecord, error) {
	now := time.Now()
	return g.storeTx(ctx, &transaction{
		ID:             tx.GetID(),
		Address:        tx.GetAddress(),
		Type:           Out,
		State:          tx.GetState(),
		Amount:         tx.GetAmount(),
		OriginalAmount: tx.GetOriginalAmount(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}, lastTxs)
}

func (g *gameDb) StoreInTx(ctx context.Context, tx TransactionRecordStore, lastTxs uint64) (TransactionRecord, error) {
	now := time.Now()
	return g.storeTx(ctx, &transaction{
		ID:             tx.GetID(),
		Address:        tx.GetAddress(),
		Type:           In,
		State:          tx.GetState(),
		Amount:         tx.GetAmount(),
		OriginalAmount: tx.GetOriginalAmount(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}, lastTxs)
}

func (g *gameDb) GetTxById(ctx context.Context, id []byte) (TransactionRecord, error) {
	tx := &transaction{}
	err := g.db.NewSelect().Model(tx).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, g.hideError(err)
	}

	return g.txFromDao(ctx, tx)
}

func (g *gameDb) GetBalanceByAddress(ctx context.Context, address string) (BalanceRecord, error) {
	acc := &account{}
	errAcc := g.db.NewSelect().Model(acc).Column("id").Where("address = ?", address).Scan(ctx)
	if errAcc != nil {
		return nil, g.hideError(errAcc)
	}

	return g.GetBalanceByPlayerID(ctx, acc.ID)
}

type pureBalance struct {
	Total             int64
	PendingWithdrawal int64
	Hold              int64
	TotalWins         int64
	TotalBonuses      int64
}

func (g *gameDb) GetBalanceByPlayerID(ctx context.Context, ID uuid.UUID) (BalanceRecord, error) {
	acc := &account{}

	errPlayer := g.db.NewSelect().Model(acc).Column("id", "address").Where("id = ?", ID).Scan(ctx)
	if errPlayer != nil {
		return nil, g.hideError(errPlayer)
	}

	var errGroup errgroup.Group
	bp := &pureBalance{}

	errGroup.Go(func() error {
		return g.db.NewSelect().Model((*transaction)(nil)).ColumnExpr("coalesce(SUM(amount), 0)").Where("address = ?", acc.Address).Where("state = ?", Finished).Scan(ctx, &bp.Total)
	})
	errGroup.Go(func() error {
		return g.db.NewSelect().Model((*transaction)(nil)).ColumnExpr("coalesce(SUM(amount), 0)").Where("address = ?", acc.Address).Where("state = ?", Pending).Where("type = ?", Out).Scan(ctx, &bp.PendingWithdrawal)
	})
	errGroup.Go(func() error {
		return g.db.NewSelect().Model((*lock)(nil)).ColumnExpr("coalesce(SUM(amount), 0)").Where("account_id = ?", acc.ID).Scan(ctx, &bp.Hold)
	})
	errGroup.Go(func() error {
		return g.db.NewSelect().Model((*win)(nil)).ColumnExpr("coalesce(SUM(amount), 0)").Where("account_id = ?", acc.ID).Scan(ctx, &bp.TotalWins)
	})
	errGroup.Go(func() error {
		return g.db.NewSelect().Model((*bonus)(nil)).ColumnExpr("coalesce(SUM(amount), 0)").Where("account_id = ?", acc.ID).Scan(ctx, &bp.TotalBonuses)
	})
	if err := errGroup.Wait(); err != nil {
		return nil, g.hideError(err)
	}

	totalWithPending := bp.Total + bp.TotalBonuses - (-bp.PendingWithdrawal) + bp.TotalWins
	if totalWithPending < bp.Hold {
		return nil, ErrInternalDBError
	}

	return &balanceRecord{
		available:         uint64(totalWithPending - bp.Hold),
		hold:              uint64(bp.Hold),
		pendingWithdrawal: uint64(-1 * bp.PendingWithdrawal),
		profit:            bp.TotalWins,
	}, nil
}
