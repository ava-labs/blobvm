// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ava-labs/blobvm/client"
)

var resolveCmd = &cobra.Command{
	Use:   "resolve [options] key",
	Short: "Reads a value at key",
	RunE:  resolveFunc,
}

func resolveFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected exactly 1 argument, got %d", len(args))
	}
	k := common.HexToHash(args[0])
	cli := client.New(uri, requestTimeout)
	_, v, vmeta, err := cli.Resolve(context.Background(), k)
	if err != nil {
		return err
	}

	color.Yellow("%v=>%q", k, v)
	hr, err := json.Marshal(vmeta)
	if err != nil {
		return err
	}
	color.Yellow("Metadata: %s", string(hr))

	color.Green("resolved %s", args[0])
	return nil
}
