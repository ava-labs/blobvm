# Blob Virtual Machine (BlobVM)

_Content-Addressable Key-Value Store w/EIP-712 Compatibility and Fee-Based Metering_

This code is similar to [SpacesVM](https://github.com/ava-labs/spacesvm) but
does away with the hierarchical, authenticated namespace, user-specified
keys, and key expiry.

## Avalanche Subnets and Custom VMs
Avalanche is a network composed of multiple sub-networks (called [subnets][Subnet]) that each contain
any number of blockchains. Each blockchain is an instance of a
[Virtual Machine (VM)](https://docs.avax.network/learn/platform-overview#virtual-machines),
much like an object in an object-oriented language is an instance of a class. That is,
the VM defines the behavior of the blockchain where it is instantiated. For example,
[Coreth (EVM)][Coreth] is a VM that is instantiated by the
[C-Chain]. Likewise, one could deploy another instance of the EVM as their own blockchain (to take
this to its logical conclusion).

## AvalancheGo Compatibility
```
[v0.0.1] AvalancheGo@v1.7.7-v1.7.9
[v0.0.2] AvalancheGo@v1.7.7-v1.7.9
[v0.0.3] AvalancheGo@v1.7.10
[v0.0.4] AvalancheGo@v1.7.11-v1.7.12
[v0.0.5] AvalancheGo@v1.7.13
```

## Introduction
Just as [Coreth] powers the [C-Chain], BlobVM can be used to power its own
blockchain in an Avalanche [Subnet]. Instead of providing a place to execute Solidity
smart contracts, however, BlobVM enables content-addressable storage of arbitrary
keys/values using any [EIP-712] compatible wallet.

### Content-Addressable Key/Value Storage
All keys in BlobVM are keccak256 hashes (each of a unique value stored in
state). The max length of values is defined in genesis but typically ranges
between 64-200KB. Any number of values can be linked together to store files in
the > 100s of MBs range (as long as you have the `BLB` to pay for it).

### [EIP-712] Compatible
The canonical digest of a BlobVM transaction is [EIP-712] compliant, so any
Web3 wallet that can sign typed data can interact with BlobVM.

**[EIP-712] compliance in this case, however, does not mean that BlobVM
is an EVM or even an EVM derivative.** BlobVM is a new Avalanche-native VM written
from scratch to optimize for storage-related operations.

## How it Works
### Set
As soon as you have some `BLB`, you can then use `SetTx` to
persist some value blob into state. This value blob will be accessible at
`keccak256(value)`. This value will live in state forever.

#### Content-Addressable Keys
To support common blockchain use cases (like NFT storage), BlobVM
supports the storage of arbitrary size files using a basic metadata file format.
You can try this out using `blob-cli set-file <filename>`.

### Resolve
When you want to view data stored in BlobVM, you call `Resolve` on the value
path: `<key>`. If you stored a file, use this command to retrieve it:
`blob-cli resolve-file <root> <destination filepath>`.

### Transfer
If you want to share some of your `BLB` with your friends, you can use
a `TransferTx` to send to any EVM-style address.

### Fees
All interactions with the BlobVM require the payment of fees (denominated in
`BLB`). The VM Genesis includes support for allocating one-off `BLB` to
different EVM-style addresses and to allocating `BLB` to an airdrop list.

Nearly all fee-related params can be tuned by the BlobVM deployer.

### Random Value Inclusion
To deter node operators from deleting data stored in state, each block header
includes the hash of a randomly selected state value concatenated with the parent blockID.
If values are pruned, node operators can't produce/verify blocks.

## Usage
_If you are interested in running the VM, not using it. Jump to [Running the
VM](#running-the-vm)._

### blob-cli
#### Install
```bash
git clone https://github.com/ava-labs/blobvm.git;
cd blobvm;
go install -v ./cmd/blob-cli;
```

#### Usage
```
BlobVM CLI

Usage:
  blob-cli [command]

Available Commands:
  activity     View recent activity on the network
  completion   Generate the autocompletion script for the specified shell
  create       Creates a new key in the default location
  genesis      Creates a new genesis in the default location
  help         Help about any command
  network      View information about this instance of the BlobVM
  resolve      Reads a value at key
  resolve-file Reads a file at a root and saves it to disk
  set          Writes a value to BlobVM
  set-file     Writes a file to BlobVM (using multiple keys)
  transfer     Transfers units to another address

Flags:
      --endpoint string           RPC endpoint for VM
  -h, --help                      help for blob-cli
      --private-key-file string   private key file path (default ".blob-cli-pk")
      --verbose                   Print verbose information about operations

Use "blob-cli [command] --help" for more information about a command.
```

##### Uploading Files
```
blob-cli set-file ~/Downloads/computer.gif -> 6fe5a52f52b34fb1e07ba90bad47811c645176d0d49ef0c7a7b4b22013f676c8
blob-cli resolve-file 6fe5a52f52b34fb1e07ba90bad47811c645176d0d49ef0c7a7b4b22013f676c8 computer_copy.gif
```

### [Golang SDK](https://github.com/ava-labs/blobvm/blob/master/client/client.go)
```golang
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
```

### Public Endpoints (`/public`)

#### blobvm.ping
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.ping",
  "params":{},
  "id": 1
}
>>> {"success":<bool>}
```

#### blobvm.network
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.network",
  "params":{},
  "id": 1
}
>>> {"networkId":<uint32>, "subnetId":<ID>, "chainId":<ID>}
```

#### blobvm.genesis
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.genesis",
  "params":{},
  "id": 1
}
>>> {"genesis":<genesis file>}
```

#### blobvm.suggestedFee
_Provide your intent and get back a transaction to sign._
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.suggestedFee",
  "params":{
    "input":<chain.Input (tx abstractor)>
  },
  "id": 1
}
>>> {"typedData":<EIP-712 compliant typed data for signing>,
>>> "totalCost":<uint64>}
```

##### chain.Input
```
{
  "type":<string>,
  "key":<string>,
  "value":<base64 encoded>,
  "to":<hex encoded>,
  "units":<uint64>
}
```

###### Transaction Types
```
set      {type,key,value}
transfer {type,to,units}
```

#### blobvm.issueTx
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.issueTx",
  "params":{
    "typedData":<EIP-712 compliant typed data>,
    "signature":<hex-encoded sig>
  },
  "id": 1
}
>>> {"txId":<ID>}
```

#### blobvm.hasTx
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.hasTx",
  "params":{
    "txId":<transaction ID>
  },
  "id": 1
}
>>> {"accepted":<bool>}
```

#### blobvm.lastAccepted
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.lastAccepted",
  "params":{},
  "id": 1
}
>>> {"height":<uint64>, "blockId":<ID>}
```

##### chain.ValueMeta
```
{
  "key":<string>,
  "valueMeta":{
    "created":<unix>,
    "updated":<unix>,
    "txId":<ID>, // where value was last set
    "size":<uint64>
  }
}
```

#### blobvm.resolve
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.resolve",
  "params":{
    "key":<string>
  },
  "id": 1
}
>>> {"exists":<bool>, "value":<base64 encoded>, "valueMeta":<chain.ValueMeta>}
```

#### blobvm.balance
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.balance",
  "params":{
    "address":<hex encoded>
  },
  "id": 1
}
>>> {"balance":<uint64>}
```

#### blobvm.recentActivity
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.recentActivity",
  "params":{},
  "id": 1
}
>>> {"activity":[<chain.Activity>,...]}
```

##### chain.Activity
```
{
  "timestamp":<unix>,
  "sender":<address>,
  "txId":<ID>,
  "type":<string>,
  "key":<string>,
  "to":<hex encoded>,
  "units":<uint64>
}
```

###### Activity Types
```
set      {timestamp,sender,txId,type,key,value}
transfer {timestamp,sender,txId,type,to,units}
```

### Advanced Public Endpoints (`/public`)

#### blobvm.suggestedRawFee
_Can use this to get the current fee rate._
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.suggestedRawFee",
  "params":{},
  "id": 1
}
>>> {"price":<uint64>,"cost":<uint64>}
```

#### blobvm.issueRawTx
```
<<< POST
{
  "jsonrpc": "2.0",
  "method": "blobvm.issueRawTx",
  "params":{
    "tx":<raw tx bytes>
  },
  "id": 1
}
>>> {"txId":<ID>}
```

## Running the VM
To build the VM (and `blob-cli`), run `./scripts/build.sh`.

### Running a local network
[`scripts/run.sh`](scripts/run.sh) automatically installs [avalanchego], sets up a local network,
and creates a `blobvm` genesis file. To build and run E2E tests, you need to set the variable `E2E` before it: `E2E=true ./scripts/run.sh 1.7.11`

_See [`tests/e2e`](tests/e2e) to see how it's set up and how its client requests are made._

```bash
# to startup a local cluster (good for development)
cd ${HOME}/go/src/github.com/ava-labs/blobvm
./scripts/run.sh 1.7.11

# to run full e2e tests and shut down cluster afterwards
cd ${HOME}/go/src/github.com/ava-labs/blobvm
E2E=true ./scripts/run.sh 1.7.11
```

```bash
# inspect cluster endpoints when ready
cat /tmp/avalanchego-v1.7.11/output.yaml
<<COMMENT
endpoint: /ext/bc/2VCAhX6vE3UnXC6s1CBPE6jJ4c4cHWMfPgCptuWS59pQ9vbeLM
logsDir: ...
pid: 12811
uris:
- http://localhost:56239
- http://localhost:56251
- http://localhost:56253
- http://localhost:56255
- http://localhost:56257
COMMENT

# ping the local cluster
curl --location --request POST 'http://localhost:61858/ext/bc/BJfusM2TpHCEfmt5i7qeE1MwVCbw5jU1TcZNz8MYUwG1PGYRL/public' \
--header 'Content-Type: application/json' \
--data-raw '{
    "jsonrpc": "2.0",
    "method": "blobvm.ping",
    "params":{},
    "id": 1
}'
<<COMMENT
{"jsonrpc":"2.0","result":{"success":true},"id":1}
COMMENT

# resolve a path
curl --location --request POST 'http://localhost:61858/ext/bc/BJfusM2TpHCEfmt5i7qeE1MwVCbw5jU1TcZNz8MYUwG1PGYRL/public' \
--header 'Content-Type: application/json' \
--data-raw '{
    "jsonrpc": "2.0",
    "method": "blobvm.resolve",
    "params":{
      "key": "0xd35882ae256d63123710cf8ab4343282d4a2c246281d3ff5e2b244744c8f7be4"
    },
    "id": 1
}'
<<COMMENT
{"jsonrpc":"2.0","result":{"exists":true, "value":"....", "valueMeta":{....}},"id":1}
COMMENT

# to terminate the cluster
kill 12811
```

### Deploying Your Own Network
Anyone can deploy their own instance of the BlobVM as a subnet on Avalanche.
All you need to do is compile it, create a genesis, and send a few txs to the
P-Chain.

You can do this by following the [subnet tutorial]
or by using the [subnet-cli].

[EIP-712]: https://eips.ethereum.org/EIPS/eip-712
[avalanchego]: https://github.com/ava-labs/avalanchego
[subnet tutorial]: https://docs.avax.network/build/tutorials/platform/subnets/create-a-subnet
[subnet-cli]: https://github.com/ava-labs/subnet-cli
[Coreth]: https://github.com/ava-labs/coreth
[C-Chain]: https://docs.avax.network/learn/platform-overview/#contract-chain-c-chain
[Subnet]: https://docs.avax.network/learn/platform-overview/#subnets

## Future Work
### Moderation
`BlobVM` does not include any built-in moderation mechanism to block/remove illicit
content. In the future, someone could implement an M-of-N governance contract
that can remove any value if it violates some code of conduct.

### Improved Access Proof
The current `AccessProof` mechanism is naive and gameable (seeded by the parent
block hash and index). In the future, someone could implement an on-chain VRF
that could be used as a more robust seed.
