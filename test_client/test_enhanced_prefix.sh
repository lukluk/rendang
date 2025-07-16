#!/bin/bash

# Test script for enhanced Redis proxy with automatic prefix functionality
# This script demonstrates how all Redis data operations automatically get prefixed

echo "Testing Enhanced Redis Proxy with Automatic Prefix Functionality"
echo "================================================================"

# Configuration
PROXY_HOST="localhost"
PROXY_PORT="6378"
REDIS_CLI="redis-cli -h $PROXY_HOST -p $PROXY_PORT"

echo "1. Testing AUTH with username (should set prefix to 'user1:')"
echo "AUTH user1 password123" | $REDIS_CLI

echo ""
echo "2. Testing String operations (should be prefixed with 'user1:')"
echo "SET mykey myvalue" | $REDIS_CLI
echo "GET mykey" | $REDIS_CLI
echo "INCR counter" | $REDIS_CLI
echo "GET counter" | $REDIS_CLI

echo ""
echo "3. Testing Hash operations"
echo "HSET user:profile name John" | $REDIS_CLI
echo "HSET user:profile age 30" | $REDIS_CLI
echo "HGETALL user:profile" | $REDIS_CLI

echo ""
echo "4. Testing List operations"
echo "LPUSH mylist item1" | $REDIS_CLI
echo "LPUSH mylist item2" | $REDIS_CLI
echo "LRANGE mylist 0 -1" | $REDIS_CLI

echo ""
echo "5. Testing Set operations"
echo "SADD myset member1" | $REDIS_CLI
echo "SADD myset member2" | $REDIS_CLI
echo "SMEMBERS myset" | $REDIS_CLI

echo ""
echo "6. Testing Sorted Set operations"
echo "ZADD leaderboard 100 player1" | $REDIS_CLI
echo "ZADD leaderboard 200 player2" | $REDIS_CLI
echo "ZRANGE leaderboard 0 -1 WITHSCORES" | $REDIS_CLI

echo ""
echo "7. Testing Multiple key operations"
echo "MSET key1 value1 key2 value2 key3 value3" | $REDIS_CLI
echo "MGET key1 key2 key3" | $REDIS_CLI

echo ""
echo "8. Testing Key operations"
echo "EXISTS mykey" | $REDIS_CLI
echo "TTL mykey" | $REDIS_CLI
echo "EXPIRE mykey 3600" | $REDIS_CLI

echo ""
echo "9. Testing with different AUTH (should change prefix to 'user2:')"
echo "AUTH user2 password456" | $REDIS_CLI
echo "SET newkey newvalue" | $REDIS_CLI
echo "GET newkey" | $REDIS_CLI

echo ""
echo "10. Testing AUTH with just password (should use password as prefix)"
echo "AUTH password789" | $REDIS_CLI
echo "SET testkey testvalue" | $REDIS_CLI
echo "GET testkey" | $REDIS_CLI

echo ""
echo "11. Testing connection without AUTH (should use default prefix)"
echo "SET defaultkey defaultvalue" | $REDIS_CLI
echo "GET defaultkey" | $REDIS_CLI

echo ""
echo "Test completed!"
echo "Check the proxy logs to see how prefixes are automatically added to all operations." 