// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/blobvm/tdata"
)

const (
	Set      = "set"
	Transfer = "transfer"
)

type Input struct {
	Typ   string         `json:"type"`
	Key   string         `json:"key"`
	Value []byte         `json:"value"`
	To    common.Address `json:"to"`
	Units uint64         `json:"units"`
}

func (i *Input) Decode() (UnsignedTransaction, error) {
	switch i.Typ {
	case Set:
		return &SetTx{
			BaseTx: &BaseTx{},
			Value:  i.Value,
		}, nil
	case Transfer:
		return &TransferTx{
			BaseTx: &BaseTx{},
			To:     i.To,
			Units:  i.Units,
		}, nil
	default:
		return nil, ErrInvalidType
	}
}

const (
	tdString  = "string"
	tdUint64  = "uint64"
	tdBytes   = "bytes"
	tdAddress = "address"

	tdBlockID = "blockID"
	tdPrice   = "price"

	tdValue = "value"
	tdUnits = "units"
	tdTo    = "to"
)

func parseUint64Message(td *tdata.TypedData, k string) (uint64, error) {
	r, ok := td.Message[k].(string)
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrTypedDataKeyMissing, k)
	}
	return strconv.ParseUint(r, 10, 64)
}

func parseBaseTx(td *tdata.TypedData) (*BaseTx, error) {
	rblockID, ok := td.Message[tdBlockID].(string)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTypedDataKeyMissing, tdBlockID)
	}
	blockID, err := ids.FromString(rblockID)
	if err != nil {
		return nil, err
	}
	magic, err := strconv.ParseUint(td.Domain.Magic, 10, 64)
	if err != nil {
		return nil, err
	}
	price, err := parseUint64Message(td, tdPrice)
	if err != nil {
		return nil, err
	}
	return &BaseTx{BlockID: blockID, Magic: magic, Price: price}, nil
}

func ParseTypedData(td *tdata.TypedData) (UnsignedTransaction, error) {
	bTx, err := parseBaseTx(td)
	if err != nil {
		return nil, err
	}

	switch td.PrimaryType {
	case Set:
		rvalue, ok := td.Message[tdValue].(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrTypedDataKeyMissing, tdValue)
		}
		value, err := hexutil.Decode(rvalue)
		if err != nil {
			return nil, err
		}
		return &SetTx{BaseTx: bTx, Value: value}, nil
	case Transfer:
		to, ok := td.Message[tdTo].(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrTypedDataKeyMissing, tdTo)
		}
		units, err := parseUint64Message(td, tdUnits)
		if err != nil {
			return nil, err
		}
		return &TransferTx{BaseTx: bTx, To: common.HexToAddress(to), Units: units}, nil
	default:
		return nil, ErrInvalidType
	}
}
