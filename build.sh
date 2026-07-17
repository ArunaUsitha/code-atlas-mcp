#!/bin/bash
# Build script for Linux / WSL

# Exit immediately if a command exits with a non-zero status
set -e

# ANSI escape codes for colors
CYAN='\033[0;36m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Colo

echo -e "${CYAN}Checking build prerequisites...${NC}"

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: 'go' command not found.${NC}"
    echo -e "Please install Go (version 1.22 or higher) in your WSL environment."
    echo -e "See https://go.dev/doc/install or run: sudo apt install golang-go"
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo -e "Found Go version: ${GO_VERSION}"

# Check if gcc is installed
if ! command -v gcc &> /dev/null; then
    echo -e "${RED}Error: 'gcc' command not found.${NC}"
    echo -e "Since tree-sitter bindings require CGO, a C compiler is required to build CodeAtlas."
    echo -e "Please install build-essential in your WSL/Linux distribution:"
    echo -e "  sudo apt-get update && sudo apt-get install -y build-essential"
    exit 1
fi

echo -e "Found GCC compiler: $(gcc --version | head -n 1)"

echo -e "${CYAN}Building CodeAtlas MCP Server...${NC}"

# Enable CGO and build the server binary
CGO_ENABLED=1 go build -o cbm-server ./cmd/cbm-server

if [ $? -eq 0 ]; then
    echo -e "${GREEN}Build Succeeded! 'cbm-server' is ready.${NC}"
    echo -e ""
    echo -e "${YELLOW}Note on Local Semantic Embeddings:${NC}"
    echo -e "To use local semantic embeddings with ONNX, you need the 'libonnxruntime.so' library."
    echo -e "If it is not present in the current directory or system path, the server will fall back to mock embeddings."
    echo -e "To set it up, you can run:"
    echo -e "  wget https://github.com/microsoft/onnxruntime/releases/download/v1.18.0/onnxruntime-linux-x64-1.18.0.tgz"
    echo -e "  tar -zxvf onnxruntime-linux-x64-1.18.0.tgz"
    echo -e "  cp onnxruntime-linux-x64-1.18.0/lib/libonnxruntime.so.1.18.0 ./libonnxruntime.so"
    echo -e "  rm -rf onnxruntime-linux-x64-1.18.0 onnxruntime-linux-x64-1.18.0.tgz"
else
    echo -e "${RED}Build Failed.${NC}"
    exit 1
fi
