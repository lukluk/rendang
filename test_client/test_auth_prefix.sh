#!/bin/bash

# Test script for Redis proxy with AUTH username prefix functionality

echo "Starting Redis proxy test..."

# Start the proxy in background
./redis-proxy &
PROXY_PID=$!

# Wait a moment for proxy to start
sleep 2

echo "Testing AUTH with username and password..."
# Test AUTH with username and password
echo -e "*3\r\n\$4\r\nAUTH\r\n\$4\r\ntest\r\n\$8\r\npassword\r\n" | nc localhost 6378

echo "Testing SET command with prefix..."
# Test SET command - should be prefixed with "test:"
echo -e "*3\r\n\$3\r\nSET\r\n\$3\r\nkey\r\n\$5\r\nvalue\r\n" | nc localhost 6378

echo "Testing GET command with prefix..."
# Test GET command - should be prefixed with "test:"
echo -e "*2\r\n\$3\r\nGET\r\n\$3\r\nkey\r\n" | nc localhost 6378

echo "Testing AUTH with just password..."
# Test AUTH with just password (should use password as prefix)
echo -e "*2\r\n\$4\r\nAUTH\r\n\$8\r\npassword2\r\n" | nc localhost 6378

echo "Testing SET command with new prefix..."
# Test SET command - should be prefixed with "password2:"
echo -e "*3\r\n\$3\r\nSET\r\n\$4\r\nkey2\r\n\$6\r\nvalue2\r\n" | nc localhost 6378

echo "Testing GET command with new prefix..."
# Test GET command - should be prefixed with "password2:"
echo -e "*2\r\n\$3\r\nGET\r\n\$4\r\nkey2\r\n" | nc localhost 6378

# Clean up
echo "Cleaning up..."
kill $PROXY_PID
wait $PROXY_PID 2>/dev/null

echo "Test completed!" 