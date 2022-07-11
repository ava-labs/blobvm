#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

if ! [[ "$0" =~ scripts/build.sh ]]; then
  echo "must be run from repository root"
  exit 255
fi

# Set default binary directory location
name="kM6h4LYe3AcEU1MB2UNg6ubzAiDAALZzpVrbX8zn3hXF6Avd8"

# Build blobvm, which is run as a subprocess
mkdir -p ./build

echo "Building blobvm in ./build/$name"
go build -o ./build/$name ./cmd/blobvm

echo "Building blob-cli in ./build/blob-cli"
go build -o ./build/blob-cli ./cmd/blob-cli
