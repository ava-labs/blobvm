// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var zeroAddress = (common.Address{})

func valueUnits(g *Genesis, size uint64) uint64 {
	return size/g.ValueUnitSize + 1
}

func ValueHash(v []byte) common.Hash {
	return common.BytesToHash(crypto.Keccak256(v))
}

func ValueHashString(v []byte) string {
	return strings.ToLower(ValueHash(v).Hex())
}
