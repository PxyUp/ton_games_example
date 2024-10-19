package database

import (
	"time"

	"github.com/xssnick/tonutils-go/tlb"
)

var (
	_ TransactionRecord = &txRecord{}
)

type TransactionRecordStore interface {
	GetID() []byte
	GetState() PaymentState
	GetType() TxType
	GetAmount() int64
	GetOriginalAmount() int64
	GetAddress() string
}

type TransactionRecord interface {
	TransactionRecordStore
	JSON() map[string]interface{}
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
}

type txRecord struct {
	id             []byte
	state          PaymentState
	address        string
	txType         TxType
	amount         int64
	createdAt      time.Time
	updatedAt      time.Time
	originalAmount int64
}

func (t *txRecord) GetCreatedAt() time.Time {
	return t.createdAt
}

func (t *txRecord) GetUpdatedAt() time.Time {
	return t.updatedAt
}

func (t *txRecord) JSON() map[string]interface{} {
	amount := t.GetAmount()
	if amount < 0 {
		amount *= -1
	}

	original := t.GetOriginalAmount()
	if original < 0 {
		original *= -1
	}
	return map[string]interface{}{
		"id":              t.GetID(),
		"amount":          tlb.FromNanoTONU(uint64(amount)).String(),
		"original_amount": tlb.FromNanoTONU(uint64(original)).String(),
		"created_at":      t.GetCreatedAt(),
		"updated_at":      t.GetUpdatedAt(),
		"type":            t.GetType(),
		"state":           t.GetState(),
	}
}

func (t *txRecord) GetAddress() string {
	return t.address
}

func (t *txRecord) GetID() []byte {
	return t.id
}

func (t *txRecord) GetState() PaymentState {
	return t.state
}

func (t *txRecord) GetType() TxType {
	return t.txType
}

func (t *txRecord) GetAmount() int64 {
	return t.amount
}

func (t *txRecord) GetOriginalAmount() int64 {
	return t.originalAmount
}
