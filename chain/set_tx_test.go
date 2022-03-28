// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestSetTx(t *testing.T) {
	t.Parallel()

	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	sender := crypto.PubkeyToAddress(priv.PublicKey)

	db := memdb.New()
	defer db.Close()

	g := DefaultGenesis()
	tt := []struct {
		utx       UnsignedTransaction
		blockTime int64
		sender    common.Address
		err       error
	}{
		{ // write with value should succeed
			utx: &SetTx{
				BaseTx: &BaseTx{
					BlockID: ids.GenerateTestID(),
				},
				Value: []byte("value"),
			},
			blockTime: 1,
			sender:    sender,
			err:       nil,
		},
		{ // write hashed value twice
			utx: &SetTx{
				BaseTx: &BaseTx{
					BlockID: ids.GenerateTestID(),
				},
				Value: []byte("value"),
			},
			blockTime: 1,
			sender:    sender,
			err:       ErrKeyExists,
		},
	}
	for i, tv := range tt {
		// Set linked value (normally done in block processing)
		id := ids.GenerateTestID()
		if tp, ok := tv.utx.(*SetTx); ok {
			if len(tp.Value) > 0 {
				if err := db.Put(PrefixTxValueKey(id), tp.Value); err != nil {
					t.Fatal(err)
				}
			}
		}
		tc := &TransactionContext{
			Genesis:   g,
			Database:  db,
			BlockTime: uint64(tv.blockTime),
			TxID:      id,
			Sender:    tv.sender,
		}
		err := tv.utx.Execute(tc)
		if !errors.Is(err, tv.err) {
			t.Fatalf("#%d: tx.Execute err expected %v, got %v", i, tv.err, err)
		}
		if tv.err != nil {
			continue
		}

		// check committed states from db
		switch tp := tv.utx.(type) {
		case *SetTx:
			k := ValueHash(tp.Value)
			vmeta, exists, err := GetValueMeta(db, k)
			if err != nil {
				t.Fatalf("#%d: failed to get meta info %v", i, err)
			}
			switch {
			case !exists:
				t.Fatalf("#%d: non-empty value should have been persisted but not found", i)
			case exists:
				if vmeta.TxID != id {
					t.Fatalf("#%d: unexpected txID %q, expected %q", i, vmeta.TxID, id)
				}
			}

			val, exists, err := GetValue(db, k)
			if err != nil {
				t.Fatalf("#%d: failed to get key info %v", i, err)
			}
			switch {
			case !exists:
				t.Fatalf("#%d: non-empty value should have been persisted but not found", i)
			case exists:
				if !bytes.Equal(tp.Value, val) {
					t.Fatalf("#%d: unexpected value %q, expected %q", i, val, tp.Value)
				}
			}
		default:
			t.Fatalf("unknown tx type %T", tv.utx)
		}
	}
}
