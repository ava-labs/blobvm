// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ava-labs/blobvm/chain"
	"github.com/ava-labs/blobvm/client"
)

var setCmd = &cobra.Command{
	Use:   "set [options] <value>",
	Short: "Writes a value to BlobVM",
	RunE:  setFunc,
}

func setFunc(cmd *cobra.Command, args []string) error {
	priv, err := crypto.LoadECDSA(privateKeyFile)
	if err != nil {
		return err
	}

	val, err := getSetOp(args)
	if err != nil {
		return err
	}

	utx := &chain.SetTx{
		BaseTx: &chain.BaseTx{},
		Value:  val,
	}

	cli := client.New(uri, requestTimeout)
	opts := []client.OpOption{client.WithPollTx()}
	if verbose {
		opts = append(opts, client.WithBalance())
	}
	if _, _, err := client.SignIssueRawTx(context.Background(), cli, utx, priv, opts...); err != nil {
		return err
	}

	color.Green("set %s", chain.ValueHash(val))
	return nil
}

func getSetOp(args []string) (val []byte, err error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected exactly 1 argument, got %d", len(args))
	}

	return []byte(args[0]), nil
}
