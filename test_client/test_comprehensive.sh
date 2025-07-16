#!/bin/bash

# Comprehensive test script for Redis proxy with different client types

echo "Starting comprehensive Redis proxy test..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if Redis server is running
check_redis_server() {
    print_status "Checking if Redis server is running..."
    if ! redis-cli -h 103.94.238.98 -p 6379 ping > /dev/null 2>&1; then
        print_error "Redis server is not running or not accessible"
        print_status "Please start Redis server first"
        exit 1
    fi
    print_status "Redis server is running"
}

# Build the proxy
build_proxy() {
    print_status "Building Redis proxy..."
    cd ..
    if ! go build -o redis-proxy main.go; then
        print_error "Failed to build Redis proxy"
        exit 1
    fi
    print_status "Redis proxy built successfully"
}

# Start the proxy
start_proxy() {
    print_status "Starting Redis proxy..."
    ./redis-proxy > proxy.log 2>&1 &
    PROXY_PID=$!
    sleep 3
    
    if ! kill -0 $PROXY_PID 2>/dev/null; then
        print_error "Failed to start Redis proxy"
        cat proxy.log
        exit 1
    fi
    print_status "Redis proxy started with PID $PROXY_PID"
}

# Test CLI client
test_cli_client() {
    print_status "Testing CLI client..."
    
    # Test AUTH with username and password
    print_status "Testing AUTH with username and password..."
    echo -e "*3\r\n\$4\r\nAUTH\r\n\$4\r\ntest\r\n\$8\r\npassword\r\n" | nc localhost 6378
    
    # Test SET command
    print_status "Testing SET command..."
    echo -e "*3\r\n\$3\r\nSET\r\n\$3\r\nkey\r\n\$5\r\nvalue\r\n" | nc localhost 6378
    
    # Test GET command
    print_status "Testing GET command..."
    echo -e "*2\r\n\$3\r\nGET\r\n\$3\r\nkey\r\n" | nc localhost 6378
    
    # Test AUTH with just password
    print_status "Testing AUTH with just password..."
    echo -e "*2\r\n\$4\r\nAUTH\r\n\$8\r\npassword2\r\n" | nc localhost 6378
    
    # Test SET command with new prefix
    print_status "Testing SET command with new prefix..."
    echo -e "*3\r\n\$3\r\nSET\r\n\$4\r\nkey2\r\n\$6\r\nvalue2\r\n" | nc localhost 6378
    
    print_status "CLI client tests completed"
}

# Test Go client
test_go_client() {
    print_status "Testing Go client..."
    
    cd test_client
    
    if ! go run main.go; then
        print_error "Go client test failed"
        cd ..
        return 1
    fi
    
    cd ..
    print_status "Go client tests completed"
}

# Test with redis-cli
test_redis_cli() {
    print_status "Testing with redis-cli..."
    
    # Test basic connection
    if ! redis-cli -p 6378 ping > /dev/null 2>&1; then
        print_error "redis-cli cannot connect to proxy"
        return 1
    fi
    
    # Test AUTH and commands
    redis-cli -p 6378 << EOF
AUTH cliuser clipass
SET clikey clivalue
GET clikey
AUTH cliuser2 clipass2
SET clikey2 clivalue2
GET clikey2
EOF
    
    print_status "redis-cli tests completed"
}

# Test with Python client (if available)
test_python_client() {
    print_status "Testing Python client..."
    
    if ! command -v python3 &> /dev/null; then
        print_warning "Python3 not found, skipping Python client test"
        return 0
    fi
    
    # Create Python test script
    cat > test_python.py << 'EOF'
import redis
import sys

try:
    # Connect to proxy
    r = redis.Redis(host='localhost', port=6378, decode_responses=True)
    
    # Test connection
    print("Testing connection...")
    r.ping()
    print("Connection successful")
    
    # Test AUTH with username and password
    print("Testing AUTH...")
    r.execute_command('AUTH', 'pyuser', 'pypass')
    print("AUTH successful")
    
    # Test SET command
    print("Testing SET...")
    r.set('pykey', 'pyvalue')
    print("SET successful")
    
    # Test GET command
    print("Testing GET...")
    value = r.get('pykey')
    print(f"GET result: {value}")
    
    # Test another AUTH
    print("Testing second AUTH...")
    r.execute_command('AUTH', 'pyuser2', 'pypass2')
    print("Second AUTH successful")
    
    # Test SET with new prefix
    print("Testing SET with new prefix...")
    r.set('pykey2', 'pyvalue2')
    print("Second SET successful")
    
    print("Python client test completed successfully")
    
except Exception as e:
    print(f"Python client test failed: {e}")
    sys.exit(1)
EOF
    
    if python3 test_python.py; then
        print_status "Python client tests completed"
    else
        print_error "Python client test failed"
        return 1
    fi
    
    # Clean up
    rm -f test_python.py
}

# Test with Node.js client (if available)
test_node_client() {
    print_status "Testing Node.js client..."
    
    if ! command -v node &> /dev/null; then
        print_warning "Node.js not found, skipping Node.js client test"
        return 0
    fi
    
    # Create Node.js test script
    cat > test_node.js << 'EOF'
const redis = require('redis');

async function testRedis() {
    try {
        // Create client
        const client = redis.createClient({
            socket: {
                host: 'localhost',
                port: 6378
            }
        });

        client.on('error', (err) => {
            console.error('Redis Client Error:', err);
            process.exit(1);
        });

        await client.connect();
        console.log('Connected to Redis proxy');

        // Test AUTH
        console.log('Testing AUTH...');
        await client.sendCommand(['AUTH', 'nodeuser', 'nodepass']);
        console.log('AUTH successful');

        // Test SET
        console.log('Testing SET...');
        await client.set('nodekey', 'nodevalue');
        console.log('SET successful');

        // Test GET
        console.log('Testing GET...');
        const value = await client.get('nodekey');
        console.log(`GET result: ${value}`);

        // Test second AUTH
        console.log('Testing second AUTH...');
        await client.sendCommand(['AUTH', 'nodeuser2', 'nodepass2']);
        console.log('Second AUTH successful');

        // Test SET with new prefix
        console.log('Testing SET with new prefix...');
        await client.set('nodekey2', 'nodevalue2');
        console.log('Second SET successful');

        await client.disconnect();
        console.log('Node.js client test completed successfully');

    } catch (error) {
        console.error('Node.js client test failed:', error);
        process.exit(1);
    }
}

testRedis();
EOF
    
    # Check if redis package is installed
    if ! npm list redis &> /dev/null; then
        print_warning "Redis npm package not found, installing..."
        npm install redis
    fi
    
    if node test_node.js; then
        print_status "Node.js client tests completed"
    else
        print_error "Node.js client test failed"
        return 1
    fi
    
    # Clean up
    rm -f test_node.js
}

# Cleanup function
cleanup() {
    print_status "Cleaning up..."
    if [ ! -z "$PROXY_PID" ]; then
        kill $PROXY_PID 2>/dev/null
        wait $PROXY_PID 2>/dev/null
    fi
    rm -f proxy.log
    print_status "Cleanup completed"
}

# Set up trap for cleanup
trap cleanup EXIT

# Main test execution
main() {
    print_status "Starting comprehensive Redis proxy test suite..."
    
    check_redis_server
    build_proxy
    start_proxy
    
    # Run all tests
    test_cli_client
    test_go_client
    test_redis_cli
    test_python_client
    test_node_client
    
    print_status "All tests completed successfully!"
    print_status "The Redis proxy is working correctly with different client types."
}

# Run main function
main 