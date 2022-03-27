// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// e2e implements the e2e tests.
package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"flag"
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	ecommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fatih/color"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/ava-labs/blobvm/chain"
	"github.com/ava-labs/blobvm/client"
	"github.com/ava-labs/blobvm/tests"
)

func TestIntegration(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "blobvm integration test suites")
}

var (
	requestTimeout  time.Duration
	clusterInfoPath string
	logDebug        bool
	shutdown        bool
)

func init() {
	flag.DurationVar(
		&requestTimeout,
		"request-timeout",
		120*time.Second,
		"timeout for transaction issuance and confirmation",
	)
	flag.StringVar(
		&clusterInfoPath,
		"cluster-info-path",
		"",
		"cluster info YAML file path (as defined in 'tests/cluster_info.go')",
	)
	flag.BoolVar(
		&logDebug,
		"log-debug",
		true,
		"set to 'false' to disable debug logging",
	)
	flag.BoolVar(
		&shutdown,
		"shutdown",
		false,
		"'true' to send SIGINT to the local cluster for shutdown",
	)
}

var (
	priv   *ecdsa.PrivateKey
	sender ecommon.Address

	clusterInfo tests.ClusterInfo
	instances   []instance

	genesis *chain.Genesis
)

type instance struct {
	uri string
	cli client.Client
}

var _ = ginkgo.BeforeSuite(func() {
	var err error
	priv, err = crypto.HexToECDSA("a1c0bd71ff64aebd666b04db0531d61479c2c031e4de38410de0609cbd6e66f0")
	gomega.Ω(err).Should(gomega.BeNil())
	sender = crypto.PubkeyToAddress(priv.PublicKey)

	gomega.Ω(clusterInfoPath).ShouldNot(gomega.BeEmpty())
	clusterInfo, err = tests.LoadClusterInfo(clusterInfoPath)
	gomega.Ω(err).Should(gomega.BeNil())

	n := len(clusterInfo.URIs)
	gomega.Ω(n).Should(gomega.BeNumerically(">", 1))

	if shutdown {
		gomega.Ω(clusterInfo.PID).Should(gomega.BeNumerically(">", 1))
	}

	instances = make([]instance, n)
	for i := range instances {
		u := clusterInfo.URIs[i] + clusterInfo.Endpoint
		instances[i] = instance{
			uri: u,
			cli: client.New(u, requestTimeout),
		}
	}
	genesis, err = instances[0].cli.Genesis(context.Background())
	gomega.Ω(err).Should(gomega.BeNil())
	color.Blue("created clients with %+v", clusterInfo)
})

var _ = ginkgo.AfterSuite(func() {
	if !shutdown {
		color.Red("skipping shutdown for PID %d", clusterInfo.PID)
		return
	}
	color.Red("shutting down local cluster on PID %d", clusterInfo.PID)
	serr := syscall.Kill(clusterInfo.PID, syscall.SIGTERM)
	color.Red("terminated local cluster on PID %d (error %v)", clusterInfo.PID, serr)
})

var _ = ginkgo.Describe("[Ping]", func() {
	ginkgo.It("can ping", func() {
		for _, inst := range instances {
			cli := inst.cli
			ok, err := cli.Ping(context.Background())
			gomega.Ω(ok).Should(gomega.BeTrue())
			gomega.Ω(err).Should(gomega.BeNil())
		}
	})
})

var _ = ginkgo.Describe("[Network]", func() {
	ginkgo.It("can get network", func() {
		sID, err := ids.FromString("24tZhrm8j8GCJRE9PomW8FaeqbgGS4UAQjJnqqn8pq5NwYSYV1")
		gomega.Ω(err).Should(gomega.BeNil())
		for _, inst := range instances {
			cli := inst.cli
			networkID, subnetID, chainID, err := cli.Network(context.Background())
			gomega.Ω(networkID).Should(gomega.Equal(uint32(1337)))
			gomega.Ω(subnetID).Should(gomega.Equal(sID))
			gomega.Ω(chainID).ShouldNot(gomega.Equal(ids.Empty))
			gomega.Ω(err).Should(gomega.BeNil())
		}
	})
})

var _ = ginkgo.Describe("[Claim/SetTx]", func() {
	ginkgo.It("get currently accepted block ID", func() {
		for _, inst := range instances {
			cli := inst.cli
			_, err := cli.Accepted(context.Background())
			gomega.Ω(err).Should(gomega.BeNil())
		}
	})

	ginkgo.It("Claim/SetTx in a single node (raw)", func() {
		space := fmt.Sprintf("0x%064x", 1000000)
		vh := chain.ValueHash(([]byte(space)))
		ginkgo.By("mine and issue ClaimTx to the first node", func() {
			claimTx := &chain.SetTx{
				BaseTx: &chain.BaseTx{},
				Value:  []byte(space),
			}

			claimed, _, meta, err := instances[0].cli.Resolve(context.Background(), space)
			gomega.Ω(err).Should(gomega.BeNil())
			gomega.Ω(claimed).Should(gomega.BeFalse())
			gomega.Ω(meta).Should(gomega.BeNil())

			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			_, _, err = client.SignIssueRawTx(
				ctx,
				instances[0].cli,
				claimTx,
				priv,
				client.WithPollTx(),
			)
			cancel()
			gomega.Ω(err).Should(gomega.BeNil())
		})

		ginkgo.By("check space to check if ClaimTx has been accepted from all nodes", func() {
			time.Sleep(40 * time.Second) // enough time to be propagated to all nodes
			for _, inst := range instances {
				color.Blue("checking space on %q", inst.uri)
				claimed, _, meta, err := inst.cli.Resolve(context.Background(), vh)
				gomega.Ω(err).To(gomega.BeNil())
				gomega.Ω(claimed).Should(gomega.BeTrue())
				fmt.Printf("%+v\n", meta.TxID)
			}
		})
	})
})
