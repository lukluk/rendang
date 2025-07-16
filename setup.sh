#!/bin/bash

# Redis Proxy Setup Script

echo "Setting up Redis Proxy..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go first."
    exit 1
fi

# Initialize Go module if not already done
if [ ! -f "go.mod" ]; then
    echo "Initializing Go module..."
    go mod init redis-proxy
fi

# Build the proxy
echo "Building Redis proxy..."
go build -o redis-proxy main.go

if [ $? -eq 0 ]; then
    echo "✅ Redis proxy built successfully!"
    echo ""
    echo "Usage:"
    echo "  ./redis-proxy"
    echo ""
    echo "Environment variables:"
    echo "  REDIS_PROXY_ADDR=:6379     # Proxy listening address (default: :6379)"
    echo "  REDIS_TARGET_ADDR=localhost:6380  # Target Redis server (default: localhost:6380)"
    echo "  REDIS_KEY_PREFIX=proxy:    # Key prefix to add (default: proxy:)"
    echo ""
    echo "Example:"
    echo "  REDIS_KEY_PREFIX=app: ./redis-proxy"
    echo ""
    echo "The proxy will automatically add the prefix to all Redis keys in commands."
else
    echo "❌ Failed to build Redis proxy"
    exit 1
fi 