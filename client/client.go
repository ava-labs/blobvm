// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package client implements "blobvm" client SDK.
package client

import (
	"context"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fatih/color"

	"github.com/ava-labs/blobvm/chain"
	"github.com/ava-labs/blobvm/tdata"
	"github.com/ava-labs/blobvm/vm"
)

// Client defines blobvm client operations.
type Client interface {
	// Pings the VM.
	Ping(ctx context.Context) (bool, error)
	// Network information about this instance of the VM
	Network(ctx context.Context) (uint32, ids.ID, ids.ID, error)

	// Returns the VM genesis.
	Genesis(ctx context.Context) (*chain.Genesis, error)
	// Accepted fetches the ID of the last accepted block.
	Accepted(ctx context.Context) (ids.ID, error)

	// Balance returns the balance of an account
	Balance(ctx context.Context, addr common.Address) (bal uint64, err error)
	// Resolve returns the value associated with a path
	Resolve(ctx context.Context, key common.Hash) (exists bool, value []byte, valueMeta *chain.ValueMeta, err error)

	// Requests the suggested price and cost from VM.
	SuggestedRawFee(ctx context.Context) (uint64, uint64, error)
	// Issues the transaction and returns the transaction ID.
	IssueRawTx(ctx context.Context, d []byte) (ids.ID, error)

	// Requests the suggested price and cost from VM, returns the input as
	// TypedData.
	SuggestedFee(ctx context.Context, i *chain.Input) (*tdata.TypedData, uint64, error)
	// Issues a human-readable transaction and returns the transaction ID.
	IssueTx(ctx context.Context, td *tdata.TypedData, sig []byte) (ids.ID, error)

	// Checks the status of the transaction, and returns "true" if confirmed.
	HasTx(ctx context.Context, id ids.ID) (bool, error)
	// Polls the transactions until its status is confirmed.
	PollTx(ctx context.Context, txID ids.ID) (confirmed bool, err error)

	// Recent actions on the network (sorted from recent to oldest)
	RecentActivity(ctx context.Context) ([]*chain.Activity, error)
}

// New creates a new client object.
func New(uri string, reqTimeout time.Duration) Client {
	req := rpc.NewEndpointRequester(
		fmt.Sprintf("%s%s", uri, vm.PublicEndpoint),
		"blobvm",
	)
	return &client{req: req}
}

type client struct {
	req rpc.EndpointRequester
}

func (cli *client) Ping(ctx context.Context) (bool, error) {
	resp := new(vm.PingReply)
	err := cli.req.SendRequest(ctx,
		"ping",
		nil,
		resp,
	)
	if err != nil {
		return false, err
	}
	return resp.Success, nil
}

func (cli *client) Network(ctx context.Context) (uint32, ids.ID, ids.ID, error) {
	resp := new(vm.NetworkReply)
	err := cli.req.SendRequest(
		ctx,
		"network",
		nil,
		resp,
	)
	if err != nil {
		return 0, ids.Empty, ids.Empty, err
	}
	return resp.NetworkID, resp.SubnetID, resp.ChainID, nil
}

func (cli *client) Genesis(ctx context.Context) (*chain.Genesis, error) {
	resp := new(vm.GenesisReply)
	err := cli.req.SendRequest(
		ctx,
		"genesis",
		nil,
		resp,
	)
	return resp.Genesis, err
}

func (cli *client) Accepted(ctx context.Context) (ids.ID, error) {
	resp := new(vm.LastAcceptedReply)
	if err := cli.req.SendRequest(
		ctx,
		"lastAccepted",
		nil,
		resp,
	); err != nil {
		color.Red("failed to get curr block %v", err)
		return ids.ID{}, err
	}
	return resp.BlockID, nil
}

func (cli *client) SuggestedRawFee(ctx context.Context) (uint64, uint64, error) {
	resp := new(vm.SuggestedRawFeeReply)
	if err := cli.req.SendRequest(
		ctx,
		"suggestedRawFee",
		nil,
		resp,
	); err != nil {
		return 0, 0, err
	}
	return resp.Price, resp.Cost, nil
}

func (cli *client) IssueRawTx(ctx context.Context, d []byte) (ids.ID, error) {
	resp := new(vm.IssueRawTxReply)
	if err := cli.req.SendRequest(
		ctx,
		"issueRawTx",
		&vm.IssueRawTxArgs{Tx: d},
		resp,
	); err != nil {
		return ids.Empty, err
	}
	return resp.TxID, nil
}

func (cli *client) HasTx(ctx context.Context, txID ids.ID) (bool, error) {
	resp := new(vm.HasTxReply)
	if err := cli.req.SendRequest(
		ctx,
		"hasTx",
		&vm.HasTxArgs{TxID: txID},
		resp,
	); err != nil {
		return false, err
	}
	return resp.Accepted, nil
}

func (cli *client) SuggestedFee(ctx context.Context, i *chain.Input) (*tdata.TypedData, uint64, error) {
	resp := new(vm.SuggestedFeeReply)
	if err := cli.req.SendRequest(
		ctx,
		"suggestedFee",
		&vm.SuggestedFeeArgs{Input: i},
		resp,
	); err != nil {
		return nil, 0, err
	}
	return resp.TypedData, resp.TotalCost, nil
}

func (cli *client) IssueTx(ctx context.Context, td *tdata.TypedData, sig []byte) (ids.ID, error) {
	resp := new(vm.IssueTxReply)
	if err := cli.req.SendRequest(
		ctx,
		"issueTx",
		&vm.IssueTxArgs{TypedData: td, Signature: sig},
		resp,
	); err != nil {
		return ids.Empty, err
	}

	return resp.TxID, nil
}

func (cli *client) PollTx(ctx context.Context, txID ids.ID) (confirmed bool, err error) {
done:
	for ctx.Err() == nil {
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			break done
		}

		confirmed, err := cli.HasTx(ctx, txID)
		if err != nil {
			color.Red("polling transaction failed %v", err)
			continue
		}
		if confirmed {
			color.Green("confirmed transaction %v", txID)
			return true, nil
		}
	}
	return false, ctx.Err()
}

func (cli *client) Resolve(ctx context.Context, key common.Hash) (bool, []byte, *chain.ValueMeta, error) {
	resp := new(vm.ResolveReply)
	if err := cli.req.SendRequest(
		ctx,
		"resolve",
		&vm.ResolveArgs{
			Key: key,
		},
		resp,
	); err != nil {
		return false, nil, nil, err
	}

	if !resp.Exists {
		return false, nil, nil, nil
	}

	if key != chain.ValueHash(resp.Value) {
		return false, nil, nil, ErrIntegrityFailure
	}
	return true, resp.Value, resp.ValueMeta, nil
}

func (cli *client) Balance(ctx context.Context, addr common.Address) (bal uint64, err error) {
	resp := new(vm.BalanceReply)
	if err = cli.req.SendRequest(
		ctx,
		"balance",
		&vm.BalanceArgs{
			Address: addr,
		},
		resp,
	); err != nil {
		return 0, err
	}
	return resp.Balance, nil
}

func (cli *client) RecentActivity(ctx context.Context) (activity []*chain.Activity, err error) {
	resp := new(vm.RecentActivityReply)
	if err = cli.req.SendRequest(
		ctx,
		"recentActivity",
		nil,
		resp,
	); err != nil {
		return nil, err
	}
	return resp.Activity, nil
}
