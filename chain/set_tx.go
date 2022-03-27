// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"strconv"

	"github.com/ava-labs/blobvm/tdata"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

var _ UnsignedTransaction = &SetTx{}

type SetTx struct {
	*BaseTx `serialize:"true" json:"baseTx"`

	Value []byte `serialize:"true" json:"value"`
}

func (s *SetTx) Execute(t *TransactionContext) error {
	g := t.Genesis
	switch {
	case len(s.Value) == 0:
		return ErrValueEmpty
	case uint64(len(s.Value)) > g.MaxValueSize:
		return ErrValueTooBig
	}

	k := []byte(ValueHash(s.Value))

	// Do not allow duplicate value setting
	_, exists, err := GetValueMeta(t.Database, k)
	if err != nil {
		return err
	}
	if exists {
		return ErrKeyExists
	}

	return PutKey(t.Database, k, &ValueMeta{
		Size:    uint64(len(s.Value)),
		TxID:    t.TxID,
		Created: t.BlockTime,
	})
}

func (s *SetTx) FeeUnits(g *Genesis) uint64 {
	// We don't subtract by 1 here because we want to charge extra for any
	// value-based interaction (even if it is small or a delete).
	return s.BaseTx.FeeUnits(g) + valueUnits(g, uint64(len(s.Value)))
}

func (s *SetTx) LoadUnits(g *Genesis) uint64 {
	return s.FeeUnits(g)
}

func (s *SetTx) Copy() UnsignedTransaction {
	value := make([]byte, len(s.Value))
	copy(value, s.Value)
	return &SetTx{
		BaseTx: s.BaseTx.Copy(),
		Value:  value,
	}
}

func (s *SetTx) TypedData() *tdata.TypedData {
	return tdata.CreateTypedData(
		s.Magic, Set,
		[]tdata.Type{
			{Name: tdValue, Type: tdBytes},
			{Name: tdPrice, Type: tdUint64},
			{Name: tdBlockID, Type: tdString},
		},
		tdata.TypedDataMessage{
			tdValue:   hexutil.Encode(s.Value),
			tdPrice:   strconv.FormatUint(s.Price, 10),
			tdBlockID: s.BlockID.String(),
		},
	)
}

func (s *SetTx) Activity() *Activity {
	return &Activity{
		Typ: Set,
		Key: ValueHash(s.Value),
	}
}
