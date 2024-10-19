package database

import (
	"context"
	"time"

	"github.com/tonkeeper/tongo"
	"github.com/xssnick/tonutils-go/tlb"
	"golang.org/x/sync/errgroup"
)

type AccountRecord interface {
	GetId() string
	GetAddress() string
	GetFriendlyAddress() string
	GetBalance() BalanceRecord
	GetCurrentGames() []*activeGame
	GetLastWins() []*winRecord
	GetCreatedAt() time.Time
	JSON() map[string]interface{}
}

type activeGame struct {
	Id     string `json:"id"`
	Amount string `json:"amount"`
}

type winRecord struct {
	GameID string `json:"game_id"`
	Amount string `json:"amount"`
}

type accountRecord struct {
	id              string
	address         string
	balanceHold     string
	friendlyAddress string
	balance         BalanceRecord
	activeGames     []*activeGame
	lastWins        []*winRecord
	createdAt       time.Time
}

func (p *accountRecord) GetAddress() string {
	return p.address
}

func (p *accountRecord) GetLastWins() []*winRecord {
	return p.lastWins
}

func (p *accountRecord) GetFriendlyAddress() string {
	return p.friendlyAddress
}

func (p *accountRecord) GetBalance() BalanceRecord {
	return p.balance
}

func (p *accountRecord) GetId() string {
	return p.id
}

func (p *accountRecord) GetCreatedAt() time.Time {
	return p.createdAt
}

func (p *accountRecord) GetCurrentGames() []*activeGame {
	return p.activeGames
}

func (a *accountRecord) JSON() map[string]interface{} {
	return map[string]interface{}{
		"id":           a.GetId(),
		"balance":      a.GetBalance().JSON(),
		"address":      a.GetFriendlyAddress(),
		"active_games": a.GetCurrentGames(),
		"last_wins":    a.GetLastWins(),
		"created_at":   a.GetCreatedAt(),
	}
}

func (g *gameDb) accountFromDao(ctx context.Context, dao *account) (*accountRecord, error) {
	friendlyAddr, err := tongo.ParseAddress(dao.Address)
	if err != nil {
		return nil, g.hideError(err)
	}

	var errGroup errgroup.Group

	var balance BalanceRecord
	errGroup.Go(func() error {
		b, errBalance := g.GetBalanceByPlayerID(ctx, dao.ID)
		if errBalance != nil {
			return errBalance
		}
		balance = b
		return nil
	})

	var currentGames []*activeGame
	errGroup.Go(func() error {
		activeGames := []*lock{}

		errLocks := g.db.NewSelect().Model(&activeGames).Column("game_id", "amount").Where("account_id = ?", dao.ID).Scan(ctx)
		if errLocks != nil {
			return errLocks
		}

		currentGames = make([]*activeGame, len(activeGames))
		for i := range activeGames {
			currentGames[i] = &activeGame{
				Id:     activeGames[i].GameID.String(),
				Amount: tlb.FromNanoTONU(activeGames[i].Amount).String(),
			}
		}

		return nil
	})

	var lastWins []*winRecord
	errGroup.Go(func() error {
		lw := []*win{}
		errWins := g.db.NewSelect().Model(&lw).Column("game_id", "amount").Where("account_id = ?", dao.ID).Order("created_at desc").Limit(10).Scan(ctx)
		if errWins != nil {
			return errWins
		}

		lastWins = make([]*winRecord, len(lw))
		for i := range lw {
			amount := lw[i].Amount
			wStr := tlb.FromNanoTONU(uint64(amount)).String()
			if amount < 0 {
				wStr = "-" + tlb.FromNanoTONU(uint64(-amount)).String()
			}
			lastWins[i] = &winRecord{
				GameID: lw[i].GameID.String(),
				Amount: wStr,
			}
		}

		return nil
	})

	errWg := errGroup.Wait()
	if errWg != nil {
		return nil, g.hideError(errWg)
	}

	ar := &accountRecord{
		id:              dao.ID.String(),
		address:         dao.Address,
		balanceHold:     tlb.FromNanoTONU(balance.Hold()).String(),
		friendlyAddress: friendlyAddr.ID.ToHuman(false, false),
		balance:         balance,
		activeGames:     currentGames,
		lastWins:        lastWins,
		createdAt:       dao.CreatedAt,
	}
	return ar, nil
}
