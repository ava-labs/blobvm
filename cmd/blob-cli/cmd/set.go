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
	Short: "Writes a value to the db",
	Long: `
Issues "SetTx" to write a key-value pair.

The space is automatically parsed with the delimiter "/".
When given a key "foo/hello", the "set" creates the transaction
with "foo" as space and "hello" as key. The space/key cannot
have more than one delimiter (e.g., "foo/hello/world" is invalid)
in order to maintain the flat key space.

It assumes the space is already claimed via "blob-cli claim".
Otherwise, the set transaction will fail.

# claims the space "hello.avax"
# "hello.avax" is the space (or namespace)
$ blob-cli claim hello.avax
<<COMMENT
success
COMMENT

# writes a key-value pair for the given namespace (space)
# by issuing "SetTx" preceded by "IssueTx" on the space:
# "hello.avax" is the space (or namespace)
# "foo" is the key
# "hello world" is the value
$ blob-cli set hello.avax/foo "hello world"
<<COMMENT
success
COMMENT

# The existing key-value cannot be overwritten by a different owner.
# The space must be claimed before it allows key-value writes.
$ blob-cli set hello.avax/foo "hello world" --private-key-file=.different-key
<<COMMENT
error
COMMENT
`,
	RunE: setFunc,
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
