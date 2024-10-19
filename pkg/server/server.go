package server

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/PxyUp/ton_games_example/pkg/config"
	"github.com/PxyUp/ton_games_example/pkg/database"
	"github.com/PxyUp/ton_games_example/pkg/logger"
	"github.com/PxyUp/ton_games_example/pkg/utils"
	"github.com/google/uuid"
	"github.com/tonkeeper/tongo"
	address2 "github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/wallet"
)

var (
	_ database.TransactionRecordStore = &txRecord{}
)

type Server interface {
	Listen(ctx context.Context, account *tlb.Account) error
	Withdrawal(ctx context.Context, address string, amount *big.Int) error
	Transfer(ctx context.Context, addr *address2.Address, amount *big.Int, comment string) error
}

type server struct {
	client ton.APIClientWrapped
	store  database.PaymentDB
	wallet *wallet.Wallet
	logger logger.Logger
}

type txRecord struct {
	id             []byte
	state          database.PaymentState
	txType         database.TxType
	amount         int64
	originalAmount int64
	address        string
}

func (t *txRecord) GetID() []byte {
	return t.id
}

func (t *txRecord) GetState() database.PaymentState {
	return t.state
}

func (t *txRecord) GetType() database.TxType {
	return t.txType
}

func (t *txRecord) GetAmount() int64 {
	return t.amount
}

func (t *txRecord) GetAddress() string {
	return t.address
}

func (t *txRecord) GetOriginalAmount() int64 {
	return t.originalAmount
}

func New(client ton.APIClientWrapped, store database.PaymentDB, wallet *wallet.Wallet, logger logger.Logger) Server {
	return &server{
		wallet: wallet,
		client: client,
		store:  store,
		logger: logger,
	}
}

func (r *server) Transfer(ctx context.Context, addr *address2.Address, amount *big.Int, comment string) error {
	return r.wallet.Transfer(ctx, addr, tlb.FromNanoTON(amount), comment)
}

func (r *server) Withdrawal(ctx context.Context, address string, amount *big.Int) error {
	addr, errAddress := address2.ParseRawAddr(address)
	if errAddress != nil {
		r.logger.Errorw("Withdrawal: cant parse address", "address", address)
		return errAddress
	}

	txId := uuid.New().String()
	_, errStore := r.store.StorePendingOutTx(ctx, &txRecord{
		id:             []byte(txId),
		address:        address,
		amount:         utils.FromOutToFull(amount),
		originalAmount: amount.Int64(),
		txType:         database.Out,
		state:          database.Pending,
	}, func() error {
		r.logger.Infow("Withdrawal: record in db created in transaction", "id", txId)
		if errTransfer := r.Transfer(ctx, addr, amount, txId); errTransfer != nil {
			r.logger.Infow("Withdrawal: error during execute transfer between account", "error", errTransfer.Error())
			return errTransfer
		}
		return nil
	})

	if errStore != nil {
		return errStore
	}

	r.logger.Infow("transfer sent by ton", "id", txId)

	return nil
}

func (s *server) processTransaction(ctx context.Context, tx *tlb.Transaction) (lstTx uint64, err error) {
	lstTx = tx.LT

	defer func() {
		if r := recover(); r != nil {
			s.logger.Errorf("recovered: %v", r)
			return
		}
	}()

	if tx.IO.In != nil {
		in := new(big.Int)

		if tx.IO.In.MsgType == tlb.MsgTypeInternal {
			in = tx.IO.In.AsInternal().Amount.Nano()

			addr, errParse := tongo.ParseAddress(tx.IO.In.AsInternal().SenderAddr().String())
			if errParse != nil {
				s.logger.Errorw("in: cant parse address", "error", errParse.Error(), "address", tx.IO.In.AsInternal().SenderAddr().String())
				return lstTx, errParse
			}
			_, errStore := s.store.StoreInTx(ctx, &txRecord{
				id:             tx.Hash,
				address:        addr.ID.String(),
				amount:         in.Int64(),
				originalAmount: in.Int64(),
				txType:         database.In,
				state:          database.Finished,
			}, lstTx)
			if errStore != nil {
				s.logger.Errorw("in: cant store transaction", "error", errStore.Error())
				return lstTx, errStore
			}
			s.logger.Infow("in: stored transaction", "id", string(tx.Hash))
			return lstTx, nil
		}

		if tx.IO.In.MsgType == tlb.MsgTypeExternalIn {
			if tx.IO.Out != nil {
				msgs, errMsgs := tx.IO.Out.ToSlice()
				if errMsgs != nil {
					return lstTx, errMsgs
				}

				if len(msgs) < 1 {
					return lstTx, nil
				}

				msg := msgs[0]
				comment := ""
				if msg.MsgType == tlb.MsgTypeInternal {
					comment = msg.AsInternal().Comment()
				}

				if comment == config.Config.NotTrackTXComment {
					s.logger.Info("skip as untraceable")
					return lstTx, nil
				}

				_, errStore := s.store.UpdateOutTxByID(ctx, []byte(comment), tx.Hash, lstTx)
				if errStore != nil {
					if errors.Is(errStore, database.ErrTxRecordNotFound) {
						addr, errParse := tongo.ParseAddress(msg.AsInternal().DestAddr().String())
						if errParse != nil {
							s.logger.Errorw("cant parse address", "error", errParse.Error(), "address", msg.AsInternal().DestAddr().String())
							return lstTx, errParse
						}

						amount := msg.AsInternal().Amount.Nano()

						_, errStoreAsNew := s.store.StoreOutTx(ctx, &txRecord{
							id:             tx.Hash,
							address:        addr.ID.String(),
							amount:         -utils.FromOutToFull(amount),
							originalAmount: -amount.Int64(),
							txType:         database.Out,
							state:          database.Finished,
						}, lstTx)
						if errStoreAsNew != nil {
							s.logger.Errorw("cant store out tx as new", "error", errStoreAsNew.Error())
							return lstTx, errStoreAsNew
						}
						s.logger.Infow("stored out transaction as new", "id", comment)
						return lstTx, nil
					}
					s.logger.Errorw("error is some internal need check", "error", errStore.Error(), "id", comment)
					return lstTx, errStore
				}
				s.logger.Infow("update store out transaction", "id", comment)
				return lstTx, nil
			}
		}
	}
	return lstTx, nil
}

func (r *server) Listen(ctx context.Context, account *tlb.Account) error {
	r.logger.Info("Starting listening transactions")
	transactions := make(chan *tlb.Transaction, 10)

	lstTx := config.Config.DefaultLastTx
	if account != nil {
		value, err := r.store.GetLastTxID(ctx)
		if err == nil && value != 0 {
			lstTx = value
			r.logger.Infow("get latest tx id", "lstTx", fmt.Sprintf("%d", lstTx))
		} else {
			r.logger.Errorw("cant get last tx id use first one", "lstTx", fmt.Sprintf("%d", lstTx))
		}
	}

	if config.Config.Local {
		<-ctx.Done()
		return nil
	}

	go r.client.SubscribeOnTransactions(ctx, r.wallet.WalletAddress(), lstTx, transactions)
	for tx := range transactions {
		_, err := r.processTransaction(ctx, tx)
		if err != nil {
			r.logger.Errorw("error during process transaction", "error", err.Error())
		}
	}

	return nil
}
