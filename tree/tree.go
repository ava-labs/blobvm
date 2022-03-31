// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tree

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ava-labs/avalanchego/utils/units"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fatih/color"

	"github.com/ava-labs/blobvm/chain"
	"github.com/ava-labs/blobvm/client"
)

type Root struct {
	Contents []byte        `json:"contents"`
	Children []common.Hash `json:"children"`
}

func Upload(
	ctx context.Context, cli client.Client, priv *ecdsa.PrivateKey,
	f io.Reader, chunkSize int,
) (common.Hash, error) {
	hashes := []common.Hash{}
	chunk := make([]byte, chunkSize)
	shouldExit := false
	opts := []client.OpOption{client.WithPollTx()}
	totalCost := uint64(0)
	uploaded := map[common.Hash]struct{}{}
	for !shouldExit {
		read, err := f.Read(chunk)
		if errors.Is(err, io.EOF) || read == 0 {
			break
		}
		if err != nil {
			return common.Hash{}, fmt.Errorf("%w: read error", err)
		}
		if read < chunkSize {
			shouldExit = true
			chunk = chunk[:read]

			// Use small file optimization
			if len(hashes) == 0 {
				break
			}
		}
		k := chain.ValueHash(chunk)
		if _, ok := uploaded[k]; ok {
			color.Yellow("already uploaded k=%s, skipping", k)
		} else if exists, _, _, err := cli.Resolve(ctx, k); err == nil && exists {
			color.Yellow("already on-chain k=%s, skipping", k)
			uploaded[k] = struct{}{}
		} else {
			tx := &chain.SetTx{
				BaseTx: &chain.BaseTx{},
				Value:  chunk,
			}
			txID, cost, err := client.SignIssueRawTx(ctx, cli, tx, priv, opts...)
			if err != nil {
				return common.Hash{}, err
			}
			totalCost += cost
			color.Yellow("uploaded k=%s txID=%s cost=%d totalCost=%d", k, txID, cost, totalCost)
			uploaded[k] = struct{}{}
		}
		hashes = append(hashes, k)
	}

	r := &Root{}
	if len(hashes) == 0 {
		if len(chunk) == 0 {
			return common.Hash{}, ErrEmpty
		}
		r.Contents = chunk
	} else {
		r.Children = hashes
	}

	rb, err := json.Marshal(r)
	if err != nil {
		return common.Hash{}, err
	}
	rk := chain.ValueHash(rb)
	tx := &chain.SetTx{
		BaseTx: &chain.BaseTx{},
		Value:  rb,
	}
	txID, cost, err := client.SignIssueRawTx(ctx, cli, tx, priv, opts...)
	if err != nil {
		return common.Hash{}, err
	}
	totalCost += cost
	color.Yellow("uploaded root=%v txID=%s cost=%d totalCost=%d", rk, txID, cost, totalCost)
	return rk, nil
}

// TODO: make multi-threaded
func Download(ctx context.Context, cli client.Client, root common.Hash, f io.Writer) error {
	exists, rb, _, err := cli.Resolve(ctx, root)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w:%v", ErrMissing, root)
	}
	var r Root
	if err := json.Unmarshal(rb, &r); err != nil {
		return err
	}

	// Use small file optimization
	if contentLen := len(r.Contents); contentLen > 0 {
		if _, err := f.Write(r.Contents); err != nil {
			return err
		}
		color.Yellow("downloaded root=%v size=%fKB", root, float64(contentLen)/units.KiB)
		return nil
	}

	if len(r.Children) == 0 {
		return ErrEmpty
	}

	amountDownloaded := 0
	for _, h := range r.Children {
		exists, b, _, err := cli.Resolve(ctx, h)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w:%s", ErrMissing, h)
		}
		if _, err := f.Write(b); err != nil {
			return err
		}
		size := len(b)
		color.Yellow("downloaded chunk=%v size=%fKB", h, float64(size)/units.KiB)
		amountDownloaded += size
	}
	color.Yellow("download complete root=%v size=%fMB", root, float64(amountDownloaded)/units.MiB)
	return nil
}
