// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// e2e implements the e2e tests.
package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	runner_sdk "github.com/ava-labs/avalanche-network-runner-sdk"
	"github.com/ava-labs/avalanche-network-runner-sdk/rpcpb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/blobvm/chain"
	"github.com/ava-labs/blobvm/client"
	eth_common "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fatih/color"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/formatter"
	"github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

func TestE2e(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "blobvm e2e test suites")
}

var (
	requestTimeout time.Duration

	networkRunnerLogLevel string
	gRPCEp                string
	gRPCGatewayEp         string

	execPath  string
	pluginDir string

	vmGenesisPath string
	outputPath    string

	mode string
)

func init() {
	flag.DurationVar(
		&requestTimeout,
		"request-timeout",
		120*time.Second,
		"timeout for transaction issuance and confirmation",
	)

	flag.StringVar(
		&networkRunnerLogLevel,
		"network-runner-log-level",
		"info",
		"gRPC server endpoint",
	)

	flag.StringVar(
		&gRPCEp,
		"network-runner-grpc-endpoint",
		"0.0.0.0:8080",
		"gRPC server endpoint",
	)
	flag.StringVar(
		&gRPCGatewayEp,
		"network-runner-grpc-gateway-endpoint",
		"0.0.0.0:8081",
		"gRPC gateway endpoint",
	)

	flag.StringVar(
		&execPath,
		"avalanchego-path",
		"",
		"avalanchego executable path",
	)

	flag.StringVar(
		&pluginDir,
		"avalanchego-plugin-dir",
		"",
		"avalanchego plugin directory",
	)

	flag.StringVar(
		&vmGenesisPath,
		"vm-genesis-path",
		"",
		"VM genesis file path",
	)

	flag.StringVar(
		&outputPath,
		"output-path",
		"",
		"output YAML path to write local cluster information",
	)

	flag.StringVar(
		&mode,
		"mode",
		"test",
		"'test' to shut down cluster after tests, 'run' to skip tests and only run without shutdown",
	)
}

const vmName = "blobvm"

var vmID ids.ID

func init() {
	// TODO: add "getVMID" util function in avalanchego and import from "avalanchego"
	b := make([]byte, 32)
	copy(b, []byte(vmName))
	var err error
	vmID, err = ids.ToID(b)
	if err != nil {
		panic(err)
	}
}

const (
	modeTest = "test"
	modeRun  = "run"
)

var (
	cli          runner_sdk.Client
	blobvmRPCEps []string
)

var _ = ginkgo.BeforeSuite(func() {
	gomega.Expect(mode).Should(gomega.Or(gomega.Equal("test"), gomega.Equal("run")))

	var err error
	cli, err = runner_sdk.New(runner_sdk.Config{
		LogLevel:    networkRunnerLogLevel,
		Endpoint:    gRPCEp,
		DialTimeout: 10 * time.Second,
	})
	gomega.Expect(err).Should(gomega.BeNil())

	ginkgo.By("calling start API via network runner", func() {
		outf("{{green}}sending 'start' with binary path:{{/}} %q (%q)\n", execPath, vmID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		resp, err := cli.Start(
			ctx,
			execPath,
			runner_sdk.WithPluginDir(pluginDir),
			runner_sdk.WithBlockchainSpecs(
				[]*rpcpb.BlockchainSpec{
					{
						VmName:  vmName,
						Genesis: vmGenesisPath,
					},
				}))
		cancel()
		gomega.Expect(err).Should(gomega.BeNil())
		outf("{{green}}successfully started:{{/}} %+v\n", resp.ClusterInfo.NodeNames)
	})

	// TODO: network runner health should imply custom VM healthiness
	// or provide a separate API for custom VM healthiness
	// "start" is async, so wait some time for cluster health
	outf("\n{{magenta}}sleeping before checking custom VM status...{{/}}: %s\n", vmID)
	time.Sleep(3 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	_, err = cli.Health(ctx)
	cancel()
	gomega.Expect(err).Should(gomega.BeNil())

	blobvmRPCEps = make([]string, 0)
	blockchainID, logsDir := "", ""

	// wait up to 5-minute for custom VM installation
	outf("\n{{magenta}}waiting for all custom VMs to report healthy...{{/}}\n")
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)
done:
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			break done
		case <-time.After(5 * time.Second):
		}

		outf("{{magenta}}checking custom VM status{{/}}\n")
		cctx, ccancel := context.WithTimeout(context.Background(), 2*time.Minute)
		resp, err := cli.Status(cctx)
		ccancel()
		gomega.Expect(err).Should(gomega.BeNil())

		// all logs are stored under root data dir
		logsDir = resp.GetClusterInfo().GetRootDataDir()

		for _, v := range resp.ClusterInfo.CustomChains {
			if v.VmId == vmID.String() {
				blockchainID = v.ChainId
				outf("{{blue}}spacesvm is ready:{{/}} %+v\n", v)
				break done
			}
		}
	}
	gomega.Expect(ctx.Err()).Should(gomega.BeNil())
	cancel()

	gomega.Expect(blockchainID).Should(gomega.Not(gomega.BeEmpty()))
	gomega.Expect(logsDir).Should(gomega.Not(gomega.BeEmpty()))

	cctx, ccancel := context.WithTimeout(context.Background(), 2*time.Minute)
	uris, err := cli.URIs(cctx)
	ccancel()
	gomega.Expect(err).Should(gomega.BeNil())
	outf("{{blue}}avalanche HTTP RPCs URIs:{{/}} %q\n", uris)

	for _, u := range uris {
		rpcEP := fmt.Sprintf("%s/ext/bc/%s/rpc", u, blockchainID)
		blobvmRPCEps = append(blobvmRPCEps, rpcEP)
		outf("{{blue}}avalanche blobvm RPC:{{/}} %q\n", rpcEP)
	}

	pid := os.Getpid()
	outf("{{blue}}{{bold}}writing output %q with PID %d{{/}}\n", outputPath, pid)
	ci := clusterInfo{
		URIs:     uris,
		Endpoint: fmt.Sprintf("/ext/bc/%s", blockchainID),
		PID:      pid,
		LogsDir:  logsDir,
	}
	gomega.Expect(ci.Save(outputPath)).Should(gomega.BeNil())

	b, err := os.ReadFile(outputPath)
	gomega.Expect(err).Should(gomega.BeNil())
	outf("\n{{blue}}$ cat %s:{{/}}\n%s\n", outputPath, string(b))

	priv, err = crypto.HexToECDSA("a1c0bd71ff64aebd666b04db0531d61479c2c031e4de38410de0609cbd6e66f0")
	gomega.Ω(err).Should(gomega.BeNil())
	sender = crypto.PubkeyToAddress(priv.PublicKey)

	instances = make([]instance, len(uris))
	for i := range uris {
		u := uris[i] + fmt.Sprintf("/ext/bc/%s", blockchainID)
		instances[i] = instance{
			uri: u,
			cli: client.New(u, requestTimeout),
		}
	}
	genesis, err = instances[0].cli.Genesis(context.Background())
	gomega.Ω(err).Should(gomega.BeNil())
})

var (
	priv   *ecdsa.PrivateKey
	sender eth_common.Address

	instances []instance

	genesis *chain.Genesis
)

type instance struct {
	uri string
	cli client.Client
}

var _ = ginkgo.AfterSuite(func() {
	switch mode {
	case modeTest:
		outf("{{red}}shutting down cluster{{/}}\n")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		_, err := cli.Stop(ctx)
		cancel()
		gomega.Expect(err).Should(gomega.BeNil())

	case modeRun:
		outf("{{yellow}}skipping shutting down cluster{{/}}\n")
	}

	outf("{{red}}shutting down client{{/}}\n")
	gomega.Expect(cli.Close()).Should(gomega.BeNil())
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
			networkID, _, chainID, err := cli.Network(context.Background())
			gomega.Ω(networkID).Should(gomega.Equal(uint32(1337)))
			gomega.Ω(chainID).ShouldNot(gomega.Equal(ids.Empty))
			gomega.Ω(err).Should(gomega.BeNil())
		}
	})
})

var _ = ginkgo.Describe("[SetTx]", func() {
	switch mode {
	case modeRun:
		outf("{{yellow}}skipping SetTx tests{{/}}\n")
		return
	}

	ginkgo.It("get currently accepted block ID", func() {
		for _, inst := range instances {
			cli := inst.cli
			_, err := cli.Accepted(context.Background())
			gomega.Ω(err).Should(gomega.BeNil())
		}
	})

	v := []byte(fmt.Sprintf("0x%064x", 1000000))
	vh := chain.ValueHash(v)
	ginkgo.It("SetTx in a single node (raw)", func() {
		ginkgo.By("issue SetTx to the first node", func() {
			setTx := &chain.SetTx{
				BaseTx: &chain.BaseTx{},
				Value:  v,
			}

			claimed, _, meta, err := instances[0].cli.Resolve(context.Background(), vh)
			gomega.Ω(err).Should(gomega.BeNil())
			gomega.Ω(claimed).Should(gomega.BeFalse())
			gomega.Ω(meta).Should(gomega.BeNil())

			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			_, _, err = client.SignIssueRawTx(
				ctx,
				instances[0].cli,
				setTx,
				priv,
				client.WithPollTx(),
			)
			cancel()
			gomega.Ω(err).Should(gomega.BeNil())
		})

		ginkgo.By("check if SetTx has been accepted from all nodes", func() {
			// enough time to be propagated to all nodes
			time.Sleep(5 * time.Second)

			for _, inst := range instances {
				color.Blue("checking %q", inst.uri)
				claimed, _, _, err := inst.cli.Resolve(context.Background(), vh)
				gomega.Ω(err).To(gomega.BeNil())
				gomega.Ω(claimed).Should(gomega.BeTrue())
			}
		})
	})

	ginkgo.It("redundant SetTx should fail", func() {
		ginkgo.By("issue SetTx to each node", func() {
			setTx := &chain.SetTx{
				BaseTx: &chain.BaseTx{},
				Value:  v,
			}
			for _, inst := range instances {
				ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
				_, _, err := client.SignIssueRawTx(
					ctx,
					inst.cli,
					setTx,
					priv,
					client.WithPollTx(),
				)
				cancel()
				gomega.Ω(err.Error()).Should(gomega.ContainSubstring(chain.ErrKeyExists.Error()))
			}
		})
	})
})

// Outputs to stdout.
//
// e.g.,
//   Out("{{green}}{{bold}}hi there %q{{/}}", "aa")
//   Out("{{magenta}}{{bold}}hi therea{{/}} {{cyan}}{{underline}}b{{/}}")
//
// ref.
// https://github.com/onsi/ginkgo/blob/v2.0.0/formatter/formatter.go#L52-L73
//
func outf(format string, args ...interface{}) {
	s := formatter.F(format, args...)
	fmt.Fprint(formatter.ColorableStdOut, s)
}

// clusterInfo represents the local cluster information.
type clusterInfo struct {
	URIs     []string `json:"uris"`
	Endpoint string   `json:"endpoint"`
	PID      int      `json:"pid"`
	LogsDir  string   `json:"logsDir"`
}

const fsModeWrite = 0o600

func (ci clusterInfo) Save(p string) error {
	ob, err := yaml.Marshal(ci)
	if err != nil {
		return err
	}
	return os.WriteFile(p, ob, fsModeWrite)
}
