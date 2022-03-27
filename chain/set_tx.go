// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"fmt"
	"strconv"

	"github.com/ava-labs/blobvm/parser"
	"github.com/ava-labs/blobvm/tdata"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

var _ UnsignedTransaction = &SetTx{}

type SetTx struct {
	*BaseTx `serialize:"true" json:"baseTx"`

	// Key is parsed from the given input, with its space removed.
	Key string `serialize:"true" json:"key"`

	// Value is written as the key-value pair to the storage. If a previous value
	// exists, it is overwritten.
	Value []byte `serialize:"true" json:"value"`
}

func (s *SetTx) Execute(t *TransactionContext) error {
	g := t.Genesis
	if err := parser.CheckContents(s.Key); err != nil {
		return err
	}
	switch {
	case len(s.Value) == 0:
		return ErrValueEmpty
	case uint64(len(s.Value)) > g.MaxValueSize:
		return ErrValueTooBig
	}

	h := valueHash(s.Value)
	if s.Key != h {
		return fmt.Errorf("%w: expected %s got %x", ErrInvalidKey, h, s.Key)
	}

	// Do not allow duplicate value setting
	_, exists, err := GetValueMeta(t.Database, []byte(s.Key))
	if err != nil {
		return err
	}
	if exists {
		return ErrKeyExists
	}

	return PutKey(t.Database, []byte(s.Key), &ValueMeta{
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
		Key:    s.Key,
		Value:  value,
	}
}

func (s *SetTx) TypedData() *tdata.TypedData {
	return tdata.CreateTypedData(
		s.Magic, Set,
		[]tdata.Type{
			{Name: tdKey, Type: tdString},
			{Name: tdValue, Type: tdBytes},
			{Name: tdPrice, Type: tdUint64},
			{Name: tdBlockID, Type: tdString},
		},
		tdata.TypedDataMessage{
			tdKey:     s.Key,
			tdValue:   hexutil.Encode(s.Value),
			tdPrice:   strconv.FormatUint(s.Price, 10),
			tdBlockID: s.BlockID.String(),
		},
	)
}

func (s *SetTx) Activity() *Activity {
	return &Activity{
		Typ: Set,
		Key: s.Key,
	}
}
