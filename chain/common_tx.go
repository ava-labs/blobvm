// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"bytes"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var zeroAddress = (common.Address{})

func (t *TransactionContext) authorized(owner common.Address) bool {
	return bytes.Equal(owner[:], t.Sender[:])
}

func valueUnits(g *Genesis, size uint64) uint64 {
	return size/g.ValueUnitSize + 1
}

func valueHash(v []byte) string {
	h := common.BytesToHash(crypto.Keccak256(v)).Hex()
	return strings.ToLower(h)
}
