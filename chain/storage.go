// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ethereum/go-ethereum/common"
	smath "github.com/ethereum/go-ethereum/common/math"
	log "github.com/inconshreveable/log15"
)

// 0x0/ (block hashes)
// 0x1/ (tx hashes)
//   -> [tx hash]=>nil
// 0x2/ (tx values)
//   -> [tx hash]=>value
// 0x3/ (item keys)
//   -> [key]
// 0x4/ (balance)
//   -> [owner]=> balance

const (
	blockPrefix   = 0x0
	txPrefix      = 0x1
	txValuePrefix = 0x2
	keyPrefix     = 0x3
	balancePrefix = 0x4

	linkedTxLRUSize = 512

	ByteDelimiter byte = '/'
)

var (
	lastAccepted  = []byte("last_accepted")
	linkedTxCache = &cache.LRU{Size: linkedTxLRUSize}
)

// [blockPrefix] + [delimiter] + [blockID]
func PrefixBlockKey(blockID ids.ID) (k []byte) {
	k = make([]byte, 2+len(blockID))
	k[0] = blockPrefix
	k[1] = ByteDelimiter
	copy(k[2:], blockID[:])
	return k
}

// [txPrefix] + [delimiter] + [txID]
func PrefixTxKey(txID ids.ID) (k []byte) {
	k = make([]byte, 2+len(txID))
	k[0] = txPrefix
	k[1] = ByteDelimiter
	copy(k[2:], txID[:])
	return k
}

// [txValuePrefix] + [delimiter] + [txID]
func PrefixTxValueKey(txID ids.ID) (k []byte) {
	k = make([]byte, 2+len(txID))
	k[0] = txValuePrefix
	k[1] = ByteDelimiter
	copy(k[2:], txID[:])
	return k
}

// Assumes [key] does not contain delimiter
// [keyPrefix] + [delimiter] + [key]
func ValueKey(key common.Hash) (k []byte) {
	k = make([]byte, 2+common.HashLength)
	k[0] = keyPrefix
	k[1] = ByteDelimiter
	copy(k[2:], key.Bytes())
	return k
}

// [balancePrefix] + [delimiter] + [address]
func PrefixBalanceKey(address common.Address) (k []byte) {
	k = make([]byte, 2+common.AddressLength)
	k[0] = balancePrefix
	k[1] = ByteDelimiter
	copy(k[2:], address[:])
	return
}

var ErrInvalidKeyFormat = errors.New("invalid key format")

func GetValueMeta(db database.KeyValueReader, key common.Hash) (*ValueMeta, bool, error) {
	// [keyPrefix] + [delimiter] + [key]
	k := ValueKey(key)
	rvmeta, err := db.Get(k)
	if errors.Is(err, database.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	vmeta := new(ValueMeta)
	if _, err := Unmarshal(rvmeta, vmeta); err != nil {
		return nil, false, err
	}
	return vmeta, true, nil
}

func GetValue(db database.KeyValueReader, key common.Hash) ([]byte, bool, error) {
	// [keyPrefix] + [delimiter] + [key]
	k := ValueKey(key)
	rvmeta, err := db.Get(k)
	if errors.Is(err, database.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	vmeta := new(ValueMeta)
	if _, err := Unmarshal(rvmeta, vmeta); err != nil {
		return nil, false, err
	}

	// Lookup stored value
	v, err := getLinkedValue(db, vmeta.TxID[:])
	if err != nil {
		return nil, false, err
	}
	return v, true, err
}

type KeyValueMeta struct {
	Key       string     `serialize:"true" json:"key"`
	ValueMeta *ValueMeta `serialize:"true" json:"valueMeta"`
}

// linkValues extracts all *SetTx.Value in [block] and replaces them with the
// corresponding txID where they were found. The extracted value is then
// written to disk.
func linkValues(db database.KeyValueWriter, block *StatelessBlock) ([]*Transaction, error) {
	g := block.vm.Genesis()
	ogTxs := make([]*Transaction, len(block.Txs))
	for i, tx := range block.Txs {
		switch t := tx.UnsignedTransaction.(type) {
		case *SetTx:
			if len(t.Value) == 0 {
				ogTxs[i] = tx
				continue
			}

			// Copy transaction for later
			cptx := tx.Copy()
			if err := cptx.Init(g); err != nil {
				return nil, err
			}
			ogTxs[i] = cptx

			if err := db.Put(PrefixTxValueKey(tx.ID()), t.Value); err != nil {
				return nil, err
			}
			t.Value = tx.id[:] // used to properly parse on restore
		default:
			ogTxs[i] = tx
		}
	}
	return ogTxs, nil
}

// restoreValues restores the unlinked values associated with all *SetTx.Value
// in [block].
func restoreValues(db database.KeyValueReader, block *StatefulBlock) error {
	for _, tx := range block.Txs {
		if t, ok := tx.UnsignedTransaction.(*SetTx); ok {
			if len(t.Value) == 0 {
				continue
			}
			txID, err := ids.ToID(t.Value)
			if err != nil {
				return err
			}
			b, err := db.Get(PrefixTxValueKey(txID))
			if err != nil {
				return err
			}
			t.Value = b
		}
	}
	return nil
}

func SetLastAccepted(db database.KeyValueWriter, block *StatelessBlock) error {
	bid := block.ID()
	if err := db.Put(lastAccepted, bid[:]); err != nil {
		return err
	}
	ogTxs, err := linkValues(db, block)
	if err != nil {
		return err
	}
	sbytes, err := Marshal(block.StatefulBlock)
	if err != nil {
		return err
	}
	if err := db.Put(PrefixBlockKey(bid), sbytes); err != nil {
		return err
	}
	// Restore the original transactions in the block in case it is cached for
	// later use.
	block.Txs = ogTxs
	return nil
}

func HasLastAccepted(db database.Database) (bool, error) {
	return db.Has(lastAccepted)
}

func GetLastAccepted(db database.KeyValueReader) (ids.ID, error) {
	v, err := db.Get(lastAccepted)
	if errors.Is(err, database.ErrNotFound) {
		return ids.ID{}, nil
	}
	if err != nil {
		return ids.ID{}, err
	}
	return ids.ToID(v)
}

func GetBlock(db database.KeyValueReader, bid ids.ID) (*StatefulBlock, error) {
	b, err := db.Get(PrefixBlockKey(bid))
	if err != nil {
		return nil, err
	}
	blk := new(StatefulBlock)
	if _, err := Unmarshal(b, blk); err != nil {
		return nil, err
	}
	if err := restoreValues(db, blk); err != nil {
		return nil, err
	}
	return blk, nil
}

// DB
func HasKey(db database.KeyValueReader, key common.Hash) (bool, error) {
	// [keyPrefix] + [delimiter] + [key]
	k := ValueKey(key)
	return db.Has(k)
}

type ValueMeta struct {
	Size    uint64 `serialize:"true" json:"size"`
	TxID    ids.ID `serialize:"true" json:"txId"`
	Created uint64 `serialize:"true" json:"created"`
}

func PutKey(db database.KeyValueWriter, key common.Hash, vmeta *ValueMeta) error {
	// [keyPrefix] + [delimiter] + [key]
	k := ValueKey(key)
	rvmeta, err := Marshal(vmeta)
	if err != nil {
		return err
	}
	return db.Put(k, rvmeta)
}

func SetTransaction(db database.KeyValueWriter, tx *Transaction) error {
	k := PrefixTxKey(tx.ID())
	return db.Put(k, nil)
}

func HasTransaction(db database.KeyValueReader, txID ids.ID) (bool, error) {
	k := PrefixTxKey(txID)
	return db.Has(k)
}

func getLinkedValue(db database.KeyValueReader, b []byte) ([]byte, error) {
	bh := string(b)
	if v, ok := linkedTxCache.Get(bh); ok {
		bytes, ok := v.([]byte)
		if !ok {
			return nil, fmt.Errorf("expected []byte but got %T", v)
		}
		return bytes, nil
	}
	txID, err := ids.ToID(b)
	if err != nil {
		return nil, err
	}
	vk := PrefixTxValueKey(txID)
	v, err := db.Get(vk)
	if err != nil {
		return nil, err
	}
	linkedTxCache.Put(bh, v)
	return v, nil
}

func GetBalance(db database.KeyValueReader, address common.Address) (uint64, error) {
	k := PrefixBalanceKey(address)
	v, err := db.Get(k)
	if errors.Is(err, database.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(v), nil
}

func SetBalance(db database.KeyValueWriter, address common.Address, bal uint64) error {
	k := PrefixBalanceKey(address)
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, bal)
	return db.Put(k, b)
}

func ModifyBalance(db database.KeyValueReaderWriter, address common.Address, add bool, change uint64) (uint64, error) {
	b, err := GetBalance(db, address)
	if err != nil {
		return 0, err
	}
	var (
		n     uint64
		xflow bool
	)
	if add {
		n, xflow = smath.SafeAdd(b, change)
	} else {
		n, xflow = smath.SafeSub(b, change)
	}
	if xflow {
		return 0, fmt.Errorf("%w: bal=%d, addr=%v, add=%t, prev=%d, change=%d", ErrInvalidBalance, b, address, add, b, change)
	}
	return n, SetBalance(db, address, n)
}

func SelectRandomValueKey(db database.Database, index uint64) common.Hash {
	seed := new(big.Int).SetUint64(index).Bytes()
	iterator := ValueHash(seed)

	startKey := ValueKey(iterator)
	baseKey := ValueKey(common.Hash{})
	cursor := db.NewIteratorWithStart(startKey)
	defer cursor.Release()
	for cursor.Next() {
		curKey := cursor.Key()
		if bytes.Compare(baseKey, curKey) < -1 { // startKey < curKey; continue search
			continue
		}
		if !bytes.HasPrefix(curKey, baseKey) { // curKey does not have prefix base key; end search
			break
		}

		// [keyPrefix] + [delimiter] + [key]
		return common.BytesToHash(curKey[2:])
	}

	// No value selected
	log.Debug("skipping value selection: no valid key")
	return common.Hash{}
}
