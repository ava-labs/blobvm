// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// integration implements the integration tests.
package integration_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/database/manager"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/units"
	avago_version "github.com/ava-labs/avalanchego/version"
	ecommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fatih/color"
	log "github.com/inconshreveable/log15"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/ava-labs/blobvm/chain"
	"github.com/ava-labs/blobvm/client"
	"github.com/ava-labs/blobvm/tdata"
	"github.com/ava-labs/blobvm/tree"
	"github.com/ava-labs/blobvm/vm"
)

func TestIntegration(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "blobvm integration test suites")
}

var (
	requestTimeout time.Duration
	vms            int
	minPrice       int64
)

func init() {
	flag.DurationVar(
		&requestTimeout,
		"request-timeout",
		120*time.Second,
		"timeout for transaction issuance and confirmation",
	)
	flag.IntVar(
		&vms,
		"vms",
		3,
		"number of VMs to create",
	)
	flag.Int64Var(
		&minPrice,
		"min-price",
		-1,
		"minimum price",
	)
}

var (
	priv   *ecdsa.PrivateKey
	sender ecommon.Address

	priv2   *ecdsa.PrivateKey
	sender2 ecommon.Address

	// when used with embedded VMs
	genesisBytes []byte
	instances    []instance

	genesis *chain.Genesis
)

type instance struct {
	nodeID     ids.NodeID
	vm         *vm.VM
	toEngine   chan common.Message
	httpServer *httptest.Server
	cli        client.Client // clients for embedded VMs
	builder    *vm.ManualBuilder
}

var _ = ginkgo.BeforeSuite(func() {
	gomega.Ω(vms).Should(gomega.BeNumerically(">", 1))

	var err error
	priv, err = crypto.GenerateKey()
	gomega.Ω(err).Should(gomega.BeNil())
	sender = crypto.PubkeyToAddress(priv.PublicKey)

	log.Debug("generated key", "addr", sender, "priv", hex.EncodeToString(crypto.FromECDSA(priv)))

	priv2, err = crypto.GenerateKey()
	gomega.Ω(err).Should(gomega.BeNil())
	sender2 = crypto.PubkeyToAddress(priv2.PublicKey)

	log.Debug("generated key", "addr", sender2, "priv", hex.EncodeToString(crypto.FromECDSA(priv2)))

	// create embedded VMs
	instances = make([]instance, vms)

	genesis = chain.DefaultGenesis()
	if minPrice >= 0 {
		genesis.MinPrice = uint64(minPrice)
	}
	genesis.Magic = 5
	genesis.BlockCostEnabled = false // disable block throttling
	genesis.CustomAllocation = []*chain.CustomAllocation{
		{
			Address: sender,
			Balance: 10000000,
		},
	}
	airdropData := []byte(fmt.Sprintf(`[{"address":"%s"}]`, sender2))
	genesis.AirdropHash = ecommon.BytesToHash(crypto.Keccak256(airdropData)).Hex()
	genesis.AirdropUnits = 1000000000
	genesisBytes, err = json.Marshal(genesis)
	gomega.Ω(err).Should(gomega.BeNil())

	networkID := uint32(1)
	subnetID := ids.GenerateTestID()
	chainID := ids.GenerateTestID()

	app := &appSender{}
	for i := range instances {
		ctx := &snow.Context{
			NetworkID: networkID,
			SubnetID:  subnetID,
			ChainID:   chainID,
			NodeID:    ids.GenerateTestNodeID(),
		}

		toEngine := make(chan common.Message, 1)
		db := manager.NewMemDB(avago_version.CurrentDatabase)

		// TODO: test appsender
		v := &vm.VM{AirdropData: airdropData}
		err := v.Initialize(
			ctx,
			db,
			genesisBytes,
			nil,
			nil,
			toEngine,
			nil,
			app,
		)
		gomega.Ω(err).Should(gomega.BeNil())

		var mb *vm.ManualBuilder
		v.SetBlockBuilder(func() vm.BlockBuilder {
			mb = v.NewManualBuilder()
			return mb
		})

		var hd map[string]*common.HTTPHandler
		hd, err = v.CreateHandlers()
		gomega.Ω(err).Should(gomega.BeNil())

		httpServer := httptest.NewServer(hd[vm.PublicEndpoint].Handler)
		instances[i] = instance{
			nodeID:     ctx.NodeID,
			vm:         v,
			toEngine:   toEngine,
			httpServer: httpServer,
			cli:        client.New(httpServer.URL, requestTimeout),
			builder:    mb,
		}
	}

	// Verify genesis allocations loaded correctly (do here otherwise test may
	// check during and it will be inaccurate)
	for _, inst := range instances {
		cli := inst.cli
		g, err := cli.Genesis(context.Background())
		gomega.Ω(err).Should(gomega.BeNil())

		for _, alloc := range g.CustomAllocation {
			bal, err := cli.Balance(context.Background(), alloc.Address)
			gomega.Ω(err).Should(gomega.BeNil())
			gomega.Ω(bal).Should(gomega.Equal(alloc.Balance))
		}
	}

	app.instances = instances
	color.Blue("created %d VMs", vms)
})

var _ = ginkgo.AfterSuite(func() {
	for _, iv := range instances {
		iv.httpServer.Close()
		err := iv.vm.Shutdown()
		gomega.Ω(err).Should(gomega.BeNil())
	}
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
		for _, inst := range instances {
			cli := inst.cli
			networkID, subnetID, chainID, err := cli.Network(context.Background())
			gomega.Ω(networkID).Should(gomega.Equal(uint32(1)))
			gomega.Ω(subnetID).ShouldNot(gomega.Equal(ids.Empty))
			gomega.Ω(chainID).ShouldNot(gomega.Equal(ids.Empty))
			gomega.Ω(err).Should(gomega.BeNil())
		}
	})
})

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))] //nolint:gosec
	}
	return string(b)
}

var _ = ginkgo.Describe("Tx Types", func() {
	ginkgo.It("ensure activity yet", func() {
		activity, err := instances[0].cli.RecentActivity(context.Background())
		gomega.Ω(err).To(gomega.BeNil())

		gomega.Ω(len(activity)).To(gomega.Equal(0))
	})

	ginkgo.It("get currently accepted block ID", func() {
		for _, inst := range instances {
			cli := inst.cli
			_, err := cli.Accepted(context.Background())
			gomega.Ω(err).Should(gomega.BeNil())
		}
	})

	v := []byte(fmt.Sprintf("0x%064x", 1000000))
	vh := chain.ValueHash(v)
	ginkgo.It("Gossip SetTx to a different node", func() {
		setTx := &chain.SetTx{
			BaseTx: &chain.BaseTx{},
			Value:  v,
		}

		ginkgo.By("issue SetTx", func() {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			_, _, err := client.SignIssueRawTx(ctx, instances[0].cli, setTx, priv)
			cancel()
			gomega.Ω(err).Should(gomega.BeNil())
		})

		ginkgo.By("send gossip from node 0 to 1", func() {
			newTxs := instances[0].vm.Mempool().NewTxs(genesis.TargetBlockSize)
			gomega.Ω(len(newTxs)).To(gomega.Equal(1))

			err := instances[0].vm.Network().GossipNewTxs(newTxs)
			gomega.Ω(err).Should(gomega.BeNil())
		})

		ginkgo.By("receive gossip in the node 1, and signal block build", func() {
			instances[1].builder.NotifyBuild()
			<-instances[1].toEngine
		})

		ginkgo.By("build block in the node 1", func() {
			blk, err := instances[1].vm.BuildBlock()
			gomega.Ω(err).To(gomega.BeNil())

			gomega.Ω(blk.Verify()).To(gomega.BeNil())
			gomega.Ω(blk.Status()).To(gomega.Equal(choices.Processing))

			err = instances[1].vm.SetPreference(blk.ID())
			gomega.Ω(err).To(gomega.BeNil())

			gomega.Ω(blk.Accept()).To(gomega.BeNil())
			gomega.Ω(blk.Status()).To(gomega.Equal(choices.Accepted))

			lastAccepted, err := instances[1].vm.LastAccepted()
			gomega.Ω(err).To(gomega.BeNil())
			gomega.Ω(lastAccepted).To(gomega.Equal(blk.ID()))
		})

		ginkgo.By("ensure key is already set", func() {
			exists, _, _, err := instances[1].cli.Resolve(context.Background(), vh)
			gomega.Ω(err).To(gomega.BeNil())
			gomega.Ω(exists).To(gomega.BeTrue())
		})

		ginkgo.By("transfer funds to other sender", func() {
			transferTx := &chain.TransferTx{
				BaseTx: &chain.BaseTx{},
				To:     sender2,
				Units:  100,
			}
			createIssueRawTx(instances[0], transferTx, priv)
			expectBlkAccept(instances[0])
		})

		v = []byte(fmt.Sprintf("0x%064x", 1000001))
		ginkgo.By("fail Gossip SetTx to a stale node when missing previous blocks", func() {
			setTx := &chain.SetTx{
				BaseTx: &chain.BaseTx{},
				Value:  v,
			}

			ginkgo.By("issue SetTx", func() {
				ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
				_, _, err := client.SignIssueRawTx(ctx, instances[0].cli, setTx, priv)
				cancel()
				gomega.Ω(err).Should(gomega.BeNil())
			})

			// since the block from previous test spec has not been replicated yet
			ginkgo.By("send gossip from node 0 to 1 should fail on server-side since 1 doesn't have the block yet", func() {
				newTxs := instances[0].vm.Mempool().NewTxs(genesis.TargetBlockSize)
				gomega.Ω(len(newTxs)).To(gomega.Equal(1))

				err := instances[0].vm.Network().GossipNewTxs(newTxs)
				gomega.Ω(err).Should(gomega.BeNil())

				// mempool in 1 should be empty, since gossip/submit failed
				gomega.Ω(instances[1].vm.Mempool().Len()).Should(gomega.Equal(0))
			})
		})

		ginkgo.By("transfer funds to other sender (simple)", func() {
			createIssueTx(instances[0], &chain.Input{
				Typ:   chain.Transfer,
				To:    sender2,
				Units: 100,
			}, priv)
			expectBlkAccept(instances[0])
		})
	})

	ginkgo.It("file ops work", func() {
		files := []string{}
		ginkgo.By("create 0-files", func() {
			for _, size := range []int64{units.KiB, 278 * units.KiB, 400 * units.KiB /* right on boundary */, 5 * units.MiB} {
				newFile, err := ioutil.TempFile("", "test")
				gomega.Ω(err).Should(gomega.BeNil())
				_, err = newFile.Seek(size-1, 0)
				gomega.Ω(err).Should(gomega.BeNil())
				_, err = newFile.Write([]byte{0})
				gomega.Ω(err).Should(gomega.BeNil())
				gomega.Ω(newFile.Close()).Should(gomega.BeNil())
				files = append(files, newFile.Name())
			}
		})

		ginkgo.By("create random files", func() {
			for _, size := range []int{units.KiB, 400 * units.KiB, 3 * units.MiB} {
				newFile, err := ioutil.TempFile("", "test")
				gomega.Ω(err).Should(gomega.BeNil())
				_, err = newFile.WriteString(RandStringRunes(size))
				gomega.Ω(err).Should(gomega.BeNil())
				gomega.Ω(newFile.Close()).Should(gomega.BeNil())
				files = append(files, newFile.Name())
			}
		})

		for _, file := range files {
			var path ecommon.Hash
			var originalFile *os.File
			var err error
			ginkgo.By("upload file", func() {
				originalFile, err = os.Open(file)
				gomega.Ω(err).Should(gomega.BeNil())

				c := make(chan struct{})
				d := make(chan struct{})
				go func() {
					asyncBlockPush(instances[0], c)
					close(d)
				}()
				path, err = tree.Upload(
					context.Background(), instances[0].cli, priv,
					originalFile, int(genesis.MaxValueSize),
				)
				gomega.Ω(err).Should(gomega.BeNil())
				close(c)
				<-d
			})

			var newFile *os.File
			ginkgo.By("download file", func() {
				newFile, err = ioutil.TempFile("", "computer")
				gomega.Ω(err).Should(gomega.BeNil())

				err = tree.Download(context.Background(), instances[0].cli, path, newFile)
				gomega.Ω(err).Should(gomega.BeNil())
			})

			ginkgo.By("compare file contents", func() {
				_, err = originalFile.Seek(0, io.SeekStart)
				gomega.Ω(err).Should(gomega.BeNil())
				rho := sha256.New()
				_, err = io.Copy(rho, originalFile)
				gomega.Ω(err).Should(gomega.BeNil())
				ho := fmt.Sprintf("%x", rho.Sum(nil))

				_, err = newFile.Seek(0, io.SeekStart)
				gomega.Ω(err).Should(gomega.BeNil())
				rhn := sha256.New()
				_, err = io.Copy(rhn, newFile)
				gomega.Ω(err).Should(gomega.BeNil())
				hn := fmt.Sprintf("%x", rhn.Sum(nil))

				gomega.Ω(ho).Should(gomega.Equal(hn))

				originalFile.Close()
				newFile.Close()
			})
		}
	})

	// TODO: full replicate blocks between nodes
})

func createIssueRawTx(i instance, utx chain.UnsignedTransaction, signer *ecdsa.PrivateKey) {
	g, err := i.cli.Genesis(context.Background())
	gomega.Ω(err).Should(gomega.BeNil())
	utx.SetMagic(g.Magic)

	la, err := i.cli.Accepted(context.Background())
	gomega.Ω(err).Should(gomega.BeNil())
	utx.SetBlockID(la)

	price, blockCost, err := i.cli.SuggestedRawFee(context.Background())
	gomega.Ω(err).Should(gomega.BeNil())
	utx.SetPrice(price + blockCost/utx.FeeUnits(g))

	dh, err := chain.DigestHash(utx)
	gomega.Ω(err).Should(gomega.BeNil())
	sig, err := chain.Sign(dh, signer)
	gomega.Ω(err).Should(gomega.BeNil())

	tx := chain.NewTx(utx, sig)
	err = tx.Init(genesis)
	gomega.Ω(err).To(gomega.BeNil())

	_, err = i.cli.IssueRawTx(context.Background(), tx.Bytes())
	gomega.Ω(err).To(gomega.BeNil())
}

func createIssueTx(i instance, input *chain.Input, signer *ecdsa.PrivateKey) {
	td, _, err := i.cli.SuggestedFee(context.Background(), input)
	gomega.Ω(err).Should(gomega.BeNil())

	dh, err := tdata.DigestHash(td)
	gomega.Ω(err).Should(gomega.BeNil())

	sig, err := chain.Sign(dh, signer)
	gomega.Ω(err).Should(gomega.BeNil())

	_, err = i.cli.IssueTx(context.Background(), td, sig)
	gomega.Ω(err).To(gomega.BeNil())
}

func asyncBlockPush(i instance, c chan struct{}) {
	timer := time.NewTicker(500 * time.Millisecond)
	for {
		select {
		case <-c:
			return
		case <-timer.C:
			// manually signal ready
			i.builder.NotifyBuild()
			// manually ack ready sig as in engine
			<-i.toEngine

			blk, err := i.vm.BuildBlock()
			if err != nil {
				continue
			}

			gomega.Ω(blk.Verify()).To(gomega.BeNil())
			gomega.Ω(blk.Status()).To(gomega.Equal(choices.Processing))

			err = i.vm.SetPreference(blk.ID())
			gomega.Ω(err).To(gomega.BeNil())

			gomega.Ω(blk.Accept()).To(gomega.BeNil())
			gomega.Ω(blk.Status()).To(gomega.Equal(choices.Accepted))

			lastAccepted, err := i.vm.LastAccepted()
			gomega.Ω(err).To(gomega.BeNil())
			gomega.Ω(lastAccepted).To(gomega.Equal(blk.ID()))
		}
	}
}

func expectBlkAccept(i instance) {
	// manually signal ready
	i.builder.NotifyBuild()
	// manually ack ready sig as in engine
	<-i.toEngine

	blk, err := i.vm.BuildBlock()
	gomega.Ω(err).To(gomega.BeNil())

	gomega.Ω(blk.Verify()).To(gomega.BeNil())
	gomega.Ω(blk.Status()).To(gomega.Equal(choices.Processing))

	err = i.vm.SetPreference(blk.ID())
	gomega.Ω(err).To(gomega.BeNil())

	gomega.Ω(blk.Accept()).To(gomega.BeNil())
	gomega.Ω(blk.Status()).To(gomega.Equal(choices.Accepted))

	lastAccepted, err := i.vm.LastAccepted()
	gomega.Ω(err).To(gomega.BeNil())
	gomega.Ω(lastAccepted).To(gomega.Equal(blk.ID()))
}

var _ common.AppSender = &appSender{}

type appSender struct {
	next      int
	instances []instance
}

func (app *appSender) SendAppGossip(ctx context.Context, appGossipBytes []byte) error {
	n := len(app.instances)
	sender := app.instances[app.next].nodeID
	app.next++
	app.next %= n
	return app.instances[app.next].vm.AppGossip(ctx, sender, appGossipBytes)
}

func (app *appSender) SendAppRequest(_ context.Context, _ ids.NodeIDSet, _ uint32, _ []byte) error {
	return nil
}

func (app *appSender) SendAppResponse(_ context.Context, _ ids.NodeID, _ uint32, _ []byte) error {
	return nil
}

func (app *appSender) SendAppGossipSpecific(_ context.Context, _ ids.NodeIDSet, _ []byte) error {
	return nil
}

func (app *appSender) SendCrossChainAppRequest(_ context.Context, _ ids.ID, _ uint32, _ []byte) error {
	return nil
}

func (app *appSender) SendCrossChainAppResponse(_ context.Context, _ ids.ID, _ uint32, _ []byte) error {
	return nil
}
