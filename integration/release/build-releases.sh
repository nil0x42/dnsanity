#!/usr/bin/env bash
#
# This script builds several variants of the DNSanity binary and places them
# under a "binaries/" subfolder. Each binary is named using the pattern:
#   dnsanity-<os>-<arch>-<version>
# Example: dnsanity-mac-x64-v1.0.0

set -euo pipefail

# Step 1: Move to the root of the git repository, so that we run "go" in the right place.
cd "$(git rev-parse --show-toplevel)"

# Step 2: Retrieve the version string by calling "go run . --version"
version="$(go run . --version | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || true)"

if [[ -z "$version" ]]; then
  echo "Failed to extract version string!"
  exit 1
fi

# Step 3: Create the output folder
mkdir -p binaries

echo "Detected version: $version"
echo "Building release binaries in the ./binaries directory ..."

# ----- Build for mac-x64 -----
echo "Building dnsanity-mac-x64-$version ..."
GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o "binaries/dnsanity-mac-x64-$version" .
echo "  => Success! Created binaries/dnsanity-mac-x64-$version"

# ----- Build for mac-arm64 -----
echo "Building dnsanity-mac-arm64-$version ..."
GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" -o "binaries/dnsanity-mac-arm64-$version" .
echo "  => Success! Created binaries/dnsanity-mac-arm64-$version"

# ----- Build for linux-x64 -----
echo "Building dnsanity-linux-x64-$version ..."
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o "binaries/dnsanity-linux-x64-$version" .
echo "  => Success! Created binaries/dnsanity-linux-x64-$version"

echo "All builds completed successfully."
echo "Generated binaries in ./binaries:"
ls -1 binaries/
