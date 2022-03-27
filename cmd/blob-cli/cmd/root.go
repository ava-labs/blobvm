// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// "blob-cli" implements blobvm client operation interface.
package cmd

import (
	"os"
	"time"

	"github.com/spf13/cobra"
)

const (
	requestTimeout = 30 * time.Second
	fsModeWrite    = 0o600
)

var (
	privateKeyFile string
	uri            string
	verbose        bool
	workDir        string

	rootCmd = &cobra.Command{
		Use:        "blob-cli",
		Short:      "SpacesVM CLI",
		SuggestFor: []string{"blob-cli", "blobcli", "spacesctl"},
	}
)

func init() {
	p, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	workDir = p

	cobra.EnablePrefixMatching = true
	rootCmd.AddCommand(
		createCmd,
		genesisCmd,
		setCmd,
		resolveCmd,
		activityCmd,
		transferCmd,
		setFileCmd,
		resolveFileCmd,
		networkCmd,
	)

	rootCmd.PersistentFlags().StringVar(
		&privateKeyFile,
		"private-key-file",
		".blob-cli-pk",
		"private key file path",
	)
	rootCmd.PersistentFlags().StringVar(
		&uri,
		"endpoint",
		"https://api.tryspaces.xyz",
		"RPC endpoint for VM",
	)
	rootCmd.PersistentFlags().BoolVar(
		&verbose,
		"verbose",
		false,
		"Print verbose information about operations",
	)
}

func Execute() error {
	return rootCmd.Execute()
}
