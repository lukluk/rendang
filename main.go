package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// RedisProxy represents a Redis proxy with automatic prefix functionality
type RedisProxy struct {
	proxyAddr     string
	targetAddr    string
	prefixes      map[net.Conn]string
	prefixMux     sync.RWMutex
	defaultPrefix string
	lastCommand   map[net.Conn]string // Track last command per connection
	lastCmdMux    sync.RWMutex        // Mutex for lastCommand
}

// NewRedisProxy creates a new Redis proxy instance
func NewRedisProxy(proxyAddr, targetAddr string) *RedisProxy {
	defaultPrefix := getEnv("REDIS_DEFAULT_PREFIX", "lukluk")
	if defaultPrefix != "" && !strings.HasSuffix(defaultPrefix, ":") {
		defaultPrefix += ":"
	}

	return &RedisProxy{
		proxyAddr:     proxyAddr,
		targetAddr:    targetAddr,
		prefixes:      make(map[net.Conn]string),
		defaultPrefix: defaultPrefix,
		lastCommand:   make(map[net.Conn]string),
	}
}

// Start begins listening for connections and proxying them
func (p *RedisProxy) Start() error {
	listener, err := net.Listen("tcp", p.proxyAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	defer listener.Close()

	log.Printf("Redis proxy listening on %s, forwarding to %s",
		p.proxyAddr, p.targetAddr)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down Redis proxy...")
		listener.Close()
	}()

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			return err
		}

		go p.handleConnection(clientConn)
	}
}

// handleConnection processes a single client connection
func (p *RedisProxy) handleConnection(clientConn net.Conn) {
	defer func() {
		clientConn.Close()
		// Clean up prefix for this connection
		p.prefixMux.Lock()
		delete(p.prefixes, clientConn)
		p.prefixMux.Unlock()
	}()

	// Connect to the actual Redis server
	serverConn, err := net.Dial("tcp", p.targetAddr)
	if err != nil {
		log.Printf("Failed to connect to Redis server: %v", err)
		return
	}
	defer serverConn.Close()

	log.Printf("New connection from %s", clientConn.RemoteAddr())

	// Set a default prefix for this connection if none is set via AUTH
	// This ensures all operations get prefixed even without explicit AUTH
	p.prefixMux.Lock()
	if _, exists := p.prefixes[clientConn]; !exists {
		if p.defaultPrefix != "" {
			p.prefixes[clientConn] = p.defaultPrefix
			log.Printf("Set configured default prefix '%s' for connection %s", p.defaultPrefix, clientConn.RemoteAddr())
		} else {
			defaultPrefix := "default:" + clientConn.RemoteAddr().String() + ":"
			p.prefixes[clientConn] = defaultPrefix
			log.Printf("Set auto-generated default prefix '%s' for connection %s", defaultPrefix, clientConn.RemoteAddr())
		}
	}
	p.prefixMux.Unlock()

	// Create bidirectional proxy with prefix modification
	done := make(chan bool, 2)

	// Client to server (with prefix modification)
	go func() {
		p.forwardWithPrefix(clientConn, serverConn, true)
		done <- true
	}()

	// Server to client (pass through)
	go func() {
		p.forwardWithPrefix(serverConn, clientConn, false)
		done <- true
	}()

	// Wait for either direction to close
	<-done
	log.Printf("Connection closed for %s", clientConn.RemoteAddr())
}

// forwardWithPrefix forwards data between connections, adding prefix to Redis commands
func (p *RedisProxy) forwardWithPrefix(src, dst net.Conn, isClientToServer bool) {
	reader := bufio.NewReader(src)
	direction := "client->server"
	if !isClientToServer {
		direction = "server->client"
	}

	for {
		// Read RESP (Redis Serialization Protocol) data
		data, err := p.readRESP(reader)
		if err != nil {
			if err != io.EOF {
				log.Printf("Read error (%s): %v", direction, err)
			}
			return
		}

		// Log the data being processed (for debugging)
		if len(data) > 0 {
			logLen := len(data)
			if logLen > 50 {
				logLen = 50
			}
			log.Printf("Processing %s data: %q", direction, data[:logLen])
		}

		if isClientToServer {
			data = p.processClientCommand(src, data)
		} else {
			// Server->client: check if last command was SCAN
			p.lastCmdMux.RLock()
			lastCmd := p.lastCommand[dst]
			p.lastCmdMux.RUnlock()
			if lastCmd == "SCAN" {
				// Filter SCAN response
				p.prefixMux.RLock()
				prefix := p.prefixes[dst]
				p.prefixMux.RUnlock()
				data = p.filterScanResponse(data, prefix)
			}
		}

		// Forward the data
		_, err = dst.Write(data)
		if err != nil {
			log.Printf("Write error (%s): %v", direction, err)
			return
		}
	}
}

// readRESP reads a complete RESP message with improved error handling
func (p *RedisProxy) readRESP(reader *bufio.Reader) ([]byte, error) {
	// Read the first byte to determine the type
	firstByte, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	switch firstByte {
	case '+': // Simple String
		return p.readSimpleString(reader, firstByte)
	case '-': // Error
		return p.readSimpleString(reader, firstByte)
	case ':': // Integer
		return p.readInteger(reader, firstByte)
	case '$': // Bulk String
		return p.readBulkString(reader, firstByte)
	case '*': // Array
		return p.readArray(reader, firstByte)
	default:
		// Log the unknown byte and try to read more context for debugging
		log.Printf("Unknown RESP type: %c (0x%02x), attempting to read context", firstByte, firstByte)

		// Try to read a few more bytes to see what's coming
		peekBytes, err := reader.Peek(10)
		if err == nil {
			log.Printf("Next bytes: %q", peekBytes)
		}

		// For now, let's try to handle this gracefully by reading until we find a valid RESP type
		// This might be some kind of protocol negotiation or malformed data
		return p.handleUnknownProtocol(reader, firstByte)
	}
}

// readSimpleString reads a simple string (status or error) with improved line ending handling
func (p *RedisProxy) readSimpleString(reader *bufio.Reader, firstByte byte) ([]byte, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	// Normalize line endings to \r\n
	if !strings.HasSuffix(line, "\r\n") {
		line = strings.TrimSuffix(line, "\n") + "\r\n"
	}

	return append([]byte{firstByte}, []byte(line)...), nil
}

// readInteger reads an integer with improved line ending handling
func (p *RedisProxy) readInteger(reader *bufio.Reader, firstByte byte) ([]byte, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	// Normalize line endings to \r\n
	if !strings.HasSuffix(line, "\r\n") {
		line = strings.TrimSuffix(line, "\n") + "\r\n"
	}

	return append([]byte{firstByte}, []byte(line)...), nil
}

// readBulkString reads a bulk string with improved error handling
func (p *RedisProxy) readBulkString(reader *bufio.Reader, firstByte byte) ([]byte, error) {
	// Read length
	lengthLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	// Normalize line endings
	if !strings.HasSuffix(lengthLine, "\r\n") {
		lengthLine = strings.TrimSuffix(lengthLine, "\n") + "\r\n"
	}

	result := append([]byte{firstByte}, []byte(lengthLine)...)

	// Parse length
	lengthStr := strings.TrimSpace(lengthLine)
	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bulk string length: %s", lengthStr)
	}

	if length == -1 {
		// Null bulk string
		return result, nil
	}

	// Read the actual string
	data := make([]byte, length)
	_, err = io.ReadFull(reader, data)
	if err != nil {
		return nil, err
	}

	// Read the trailing \r\n
	crlf := make([]byte, 2)
	_, err = io.ReadFull(reader, crlf)
	if err != nil {
		return nil, err
	}

	return append(result, append(data, crlf...)...), nil
}

// readArray reads an array with improved error handling
func (p *RedisProxy) readArray(reader *bufio.Reader, firstByte byte) ([]byte, error) {
	// Read array length
	lengthLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	// Normalize line endings
	if !strings.HasSuffix(lengthLine, "\r\n") {
		lengthLine = strings.TrimSuffix(lengthLine, "\n") + "\r\n"
	}

	result := append([]byte{firstByte}, []byte(lengthLine)...)

	// Parse length
	lengthStr := strings.TrimSpace(lengthLine)
	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return nil, fmt.Errorf("invalid array length: %s", lengthStr)
	}

	if length == -1 {
		// Null array
		return result, nil
	}

	// Read each element
	for i := 0; i < length; i++ {
		element, err := p.readRESP(reader)
		if err != nil {
			return nil, err
		}
		result = append(result, element...)
	}

	return result, nil
}

// handleUnknownProtocol attempts to handle unknown protocol data gracefully
func (p *RedisProxy) handleUnknownProtocol(reader *bufio.Reader, firstByte byte) ([]byte, error) {
	// Try to read a line to see if this is some kind of text-based protocol
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read unknown protocol data: %v", err)
	}

	log.Printf("Unknown protocol data: %c%s", firstByte, line)

	// If this looks like a text-based protocol, try to forward it as-is
	// This might be some kind of protocol negotiation or handshake
	data := append([]byte{firstByte}, []byte(line)...)

	// Try to read more data until we find a valid RESP type or connection ends
	for {
		// Peek at the next byte
		peekBytes, err := reader.Peek(1)
		if err != nil {
			break
		}

		nextByte := peekBytes[0]
		if nextByte == '+' || nextByte == '-' || nextByte == ':' || nextByte == '$' || nextByte == '*' {
			// Found a valid RESP type, stop here
			log.Printf("Found valid RESP type after unknown protocol data: %c", nextByte)
			break
		}

		// Read and forward this byte
		b, err := reader.ReadByte()
		if err != nil {
			break
		}
		data = append(data, b)
	}

	return data, nil
}

// processClientCommand processes client commands, handling AUTH and adding prefixes
func (p *RedisProxy) processClientCommand(clientConn net.Conn, data []byte) []byte {
	// Parse command for tracking
	args, _ := p.parseRESPArray(data)
	if len(args) > 0 {
		cmd := strings.ToUpper(args[0])
		p.lastCmdMux.Lock()
		p.lastCommand[clientConn] = cmd
		p.lastCmdMux.Unlock()
	}
	// Check if this is a blocked command
	log.Printf("Processing client command: %q", data)
	if p.isBlockedCommand(data) {
		log.Printf("Blocked command from %s", clientConn.RemoteAddr())
		return p.createErrorResponse("ERR Command not allowed")
	}

	// Check if this is an AUTH command
	if p.isAuthCommand(data) {
		username := p.extractAuthUsername(data)
		log.Printf("Extracted username: %s", username)
		if username != "" {
			prefix := username + ":"
			p.prefixMux.Lock()
			p.prefixes[clientConn] = prefix
			p.prefixMux.Unlock()
			log.Printf("Set prefix '%s' for connection %s", prefix, clientConn.RemoteAddr())
		} else {
			// If no username found, try to use a default prefix or the password
			password := p.extractAuthPassword(data)
			if password != "" {
				prefix := password + ":"
				p.prefixMux.Lock()
				p.prefixes[clientConn] = prefix
				p.prefixMux.Unlock()
				log.Printf("Set password-based prefix '%s' for connection %s", prefix, clientConn.RemoteAddr())
			}
		}
		return data
	}

	// Add prefix to keys for other commands
	return p.addPrefixToKeys(clientConn, data)
}

// isBlockedCommand checks if the command is in the blocked commands list
func (p *RedisProxy) isBlockedCommand(data []byte) bool {
	if len(data) == 0 || data[0] != '*' {
		return false
	}

	// Split by both \r\n and \n to handle different line endings
	return strings.Contains(string(data), "flush")
}

// createErrorResponse creates a Redis error response
func (p *RedisProxy) createErrorResponse(message string) []byte {
	return []byte("-" + message + "\r\n")
}

// isAuthCommand checks if the command is an AUTH command with proper RESP parsing
func (p *RedisProxy) isAuthCommand(data []byte) bool {
	if len(data) == 0 || data[0] != '*' {
		return false
	}

	// Parse the RESP array to get the command
	args, err := p.parseRESPArray(data)
	if err != nil {
		return false
	}

	if len(args) == 0 {
		return false
	}

	command := strings.ToUpper(args[0])
	return command == "AUTH"
}

// extractAuthUsername extracts the username from an AUTH command with proper RESP parsing
func (p *RedisProxy) extractAuthUsername(data []byte) string {
	args, err := p.parseRESPArray(data)
	if err != nil {
		return ""
	}

	// AUTH command formats:
	// 1. AUTH password: *2\r\n$4\r\nAUTH\r\n$password_length\r\npassword\r\n
	// 2. AUTH username password: *3\r\n$4\r\nAUTH\r\n$username_length\r\nusername\r\n$password_length\r\npassword\r\n

	if len(args) >= 3 && strings.ToUpper(args[0]) == "AUTH" {
		// AUTH with username and password
		return args[1]
	}

	return ""
}

// extractAuthPassword extracts the password from an AUTH command with proper RESP parsing
func (p *RedisProxy) extractAuthPassword(data []byte) string {
	args, err := p.parseRESPArray(data)
	if err != nil {
		return ""
	}

	// AUTH command formats:
	// 1. AUTH password: *2\r\n$4\r\nAUTH\r\n$password_length\r\npassword\r\n
	// 2. AUTH username password: *3\r\n$4\r\nAUTH\r\n$username_length\r\nusername\r\n$password_length\r\npassword\r\n

	if len(args) >= 3 && strings.ToUpper(args[0]) == "AUTH" {
		// AUTH with username and password
		return args[2]
	} else if len(args) >= 2 && strings.ToUpper(args[0]) == "AUTH" {
		// AUTH with just password
		return args[1]
	}

	return ""
}

// parseRESPArray parses a RESP array and returns the arguments as strings
func (p *RedisProxy) parseRESPArray(data []byte) ([]string, error) {
	if len(data) == 0 || data[0] != '*' {
		return nil, fmt.Errorf("not a RESP array")
	}

	// Find the first \r\n to get the array length
	crlfIndex := bytes.Index(data, []byte("\r\n"))
	if crlfIndex == -1 {
		return nil, fmt.Errorf("invalid RESP array format")
	}

	// Parse array length
	lengthStr := string(data[1:crlfIndex])
	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return nil, fmt.Errorf("invalid array length: %s", lengthStr)
	}

	args := make([]string, 0, length)
	pos := crlfIndex + 2 // Skip past \r\n

	// Parse each element in the array
	for i := 0; i < length; i++ {
		if pos >= len(data) {
			return nil, fmt.Errorf("unexpected end of data")
		}

		if data[pos] != '$' {
			return nil, fmt.Errorf("expected bulk string, got %c", data[pos])
		}

		// Find the \r\n after the length
		crlfIndex = bytes.Index(data[pos:], []byte("\r\n"))
		if crlfIndex == -1 {
			return nil, fmt.Errorf("invalid bulk string format")
		}
		crlfIndex += pos

		// Parse string length
		strLengthStr := string(data[pos+1 : crlfIndex])
		strLength, err := strconv.Atoi(strLengthStr)
		if err != nil {
			return nil, fmt.Errorf("invalid string length: %s", strLengthStr)
		}

		pos = crlfIndex + 2 // Skip past \r\n

		// Read the string content
		if pos+strLength+2 > len(data) {
			return nil, fmt.Errorf("string content exceeds data length")
		}

		arg := string(data[pos : pos+strLength])
		args = append(args, arg)

		pos += strLength + 2 // Skip past string content and \r\n
	}

	return args, nil
}

// parseRESP recursively parses a RESP value and returns it as interface{} (string or []interface{})
func (p *RedisProxy) parseRESP(data []byte) (interface{}, int, error) {
	if len(data) == 0 {
		return nil, 0, fmt.Errorf("empty data")
	}
	switch data[0] {
	case '*': // Array
		// Find array length
		crlf := bytes.Index(data, []byte("\r\n"))
		if crlf == -1 {
			return nil, 0, fmt.Errorf("invalid array header")
		}
		length, err := strconv.Atoi(string(data[1:crlf]))
		if err != nil {
			return nil, 0, fmt.Errorf("invalid array length")
		}
		arr := make([]interface{}, 0, length)
		pos := crlf + 2
		for i := 0; i < length; i++ {
			v, n, err := p.parseRESP(data[pos:])
			if err != nil {
				return nil, 0, err
			}
			arr = append(arr, v)
			pos += n
		}
		return arr, pos, nil
	case '$': // Bulk string
		crlf := bytes.Index(data, []byte("\r\n"))
		if crlf == -1 {
			return nil, 0, fmt.Errorf("invalid bulk string header")
		}
		strlen, err := strconv.Atoi(string(data[1:crlf]))
		if err != nil {
			return nil, 0, fmt.Errorf("invalid bulk string length")
		}
		if strlen == -1 {
			return nil, crlf + 2, nil // Null bulk string
		}
		start := crlf + 2
		end := start + strlen
		if end+2 > len(data) {
			return nil, 0, fmt.Errorf("bulk string out of bounds")
		}
		str := string(data[start:end])
		return str, end + 2, nil
	default:
		return nil, 0, fmt.Errorf("unsupported RESP type: %c", data[0])
	}
}

// buildRESPArray builds a RESP array from []interface{} (strings or []interface{})
func (p *RedisProxy) buildRESPArray(arr []interface{}) []byte {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("*%d\r\n", len(arr)))
	for _, v := range arr {
		switch vv := v.(type) {
		case string:
			buf.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(vv), vv))
		case []interface{}:
			buf.Write(p.buildRESPArray(vv))
		}
	}
	return buf.Bytes()
}

// addPrefixToKeys adds the configured prefix to Redis keys in commands with proper RESP parsing
func (p *RedisProxy) addPrefixToKeys(clientConn net.Conn, data []byte) []byte {
	// Get prefix for this connection
	p.prefixMux.RLock()
	prefix, exists := p.prefixes[clientConn]
	p.prefixMux.RUnlock()

	if !exists || prefix == "" {
		return data
	}

	// Only process arrays (commands)
	if len(data) == 0 || data[0] != '*' {
		return data
	}

	// Parse the RESP array to get the command and arguments
	args, err := p.parseRESPArray(data)
	if err != nil {
		return data
	}

	if len(args) == 0 {
		return data
	}

	command := strings.ToUpper(args[0])

	// Comprehensive list of all Redis commands that operate on keys
	// This includes all data structure operations
	keyCommands := map[string]bool{
		// String operations
		"GET": true, "SET": true, "SETEX": true, "SETNX": true, "MSET": true, "MGET": true,
		"INCR": true, "DECR": true, "INCRBY": true, "DECRBY": true, "INCRBYFLOAT": true,
		"APPEND": true, "STRLEN": true, "GETRANGE": true, "SETRANGE": true,
		"GETSET": true, "PSETEX": true, "MSETNX": true,

		// Hash operations
		"HGET": true, "HSET": true, "HSETNX": true, "HMSET": true, "HMGET": true,
		"HGETALL": true, "HDEL": true, "HEXISTS": true, "HLEN": true, "HKEYS": true,
		"HVALS": true, "HINCRBY": true, "HINCRBYFLOAT": true, "HSCAN": true,

		// List operations
		"LPUSH": true, "RPUSH": true, "LPOP": true, "RPOP": true, "LLEN": true,
		"LINDEX": true, "LSET": true, "LRANGE": true, "LTRIM": true, "LREM": true,
		"LPUSHX": true, "RPUSHX": true, "LINSERT": true, "RPOPLPUSH": true,
		"BLPOP": true, "BRPOP": true, "BRPOPLPUSH": true,

		// Set operations
		"SADD": true, "SREM": true, "SMEMBERS": true, "SISMEMBER": true, "SCARD": true,
		"SPOP": true, "SRANDMEMBER": true, "SMOVE": true, "SINTER": true, "SINTERSTORE": true,
		"SUNION": true, "SUNIONSTORE": true, "SDIFF": true, "SDIFFSTORE": true,
		"SSCAN": true,

		// Sorted Set operations
		"ZADD": true, "ZREM": true, "ZSCORE": true, "ZINCRBY": true, "ZCARD": true,
		"ZRANGE": true, "ZREVRANGE": true, "ZRANGEBYSCORE": true, "ZREVRANGEBYSCORE": true,
		"ZCOUNT": true, "ZRANK": true, "ZREVRANK": true, "ZREMRANGEBYRANK": true,
		"ZREMRANGEBYSCORE": true, "ZRANGEBYLEX": true, "ZREVRANGEBYLEX": true,
		"ZREMRANGEBYLEX": true, "ZLEXCOUNT": true, "ZSCAN": true,

		// Key operations
		"DEL": true, "EXISTS": true, "EXPIRE": true, "EXPIREAT": true, "TTL": true,
		"PERSIST": true, "PEXPIRE": true, "PEXPIREAT": true, "PTTL": true,
		"RENAME": true, "RENAMENX": true, "TYPE": true, "RANDOMKEY": true,
		"DUMP": true, "RESTORE": true, "MOVE": true, "OBJECT": true,

		// Transaction operations
		"MULTI": true, "EXEC": true, "DISCARD": true, "WATCH": true, "UNWATCH": true,

		// Script operations
		"EVAL": true, "EVALSHA": true, "SCRIPT": true,

		// Stream operations
		"XADD": true, "XREAD": true, "XREADGROUP": true, "XRANGE": true, "XREVRANGE": true,
		"XLEN": true, "XDEL": true, "XTRIM": true, "XACK": true, "XCLAIM": true,
		"XPENDING": true, "XGROUP": true, "XINFO": true,

		// HyperLogLog operations
		"PFADD": true, "PFCOUNT": true, "PFMERGE": true,

		// Bitmap operations
		"SETBIT": true, "GETBIT": true, "BITCOUNT": true, "BITPOS": true,
		"BITOP": true, "BITFIELD": true,

		// Geo operations
		"GEOADD": true, "GEOPOS": true, "GEODIST": true, "GEORADIUS": true,
		"GEORADIUSBYMEMBER": true, "GEOHASH": true,

		// Pub/Sub operations
		"PUBLISH": true, "SUBSCRIBE": true, "UNSUBSCRIBE": true, "PSUBSCRIBE": true,
		"PUNSUBSCRIBE": true, "PUBSUB": true,
	}

	// Check if this is a key command
	if !keyCommands[command] {
		return data
	}

	// Special handling for commands that don't need prefixing
	noPrefixCommands := map[string]bool{
		"AUTH": true, "PING": true, "ECHO": true, "SELECT": true, "FLUSHDB": true,
		"FLUSHALL": true, "INFO": true, "CONFIG": true, "CLIENT": true, "SLOWLOG": true,
		"MONITOR": true, "SYNC": true, "PSYNC": true, "REPLCONF": true,
	}

	if noPrefixCommands[command] {
		return data
	}

	// Handle different command patterns
	switch command {
	case "MSET", "MGET", "HMSET", "HMGET":
		// These commands take multiple key-value pairs
		return p.addPrefixToMultipleKeysRESP(data, args, prefix, 1)
	case "SINTER", "SUNION", "SDIFF", "SINTERSTORE", "SUNIONSTORE", "SDIFFSTORE":
		// Set operations with multiple keys
		return p.addPrefixToMultipleKeysRESP(data, args, prefix, 1)
	case "ZINTERSTORE", "ZUNIONSTORE":
		// Sorted set operations with multiple keys
		return p.addPrefixToMultipleKeysRESP(data, args, prefix, 2)
	case "BITOP":
		// BITOP operation destination key + source keys
		return p.addPrefixToMultipleKeysRESP(data, args, prefix, 1)
	case "PFMERGE":
		// HyperLogLog merge with multiple keys
		return p.addPrefixToMultipleKeysRESP(data, args, prefix, 1)
	case "XREAD", "XREADGROUP":
		// Stream read operations with multiple streams
		return p.addPrefixToMultipleKeysRESP(data, args, prefix, 1)
	case "RENAME":
		// RENAME takes two keys
		return p.addPrefixToMultipleKeysRESP(data, args, prefix, 1)
	case "RENAMENX":
		// RENAMENX takes two keys
		return p.addPrefixToMultipleKeysRESP(data, args, prefix, 1)
	case "MOVE":
		// MOVE takes key and database number
		return p.addPrefixToSingleKeyRESP(data, args, prefix, 1)
	case "EVAL", "EVALSHA":
		// EVAL/EVALSHA: script, numkeys, key1, key2, ..., arg1, arg2, ...
		return p.addPrefixToEvalKeysRESP(data, args, prefix)
	default:
		// For most commands, prefix the first key argument
		return p.addPrefixToSingleKeyRESP(data, args, prefix, 1)
	}
}

// addPrefixToSingleKeyRESP adds prefix to a single key at the specified position using RESP parsing
func (p *RedisProxy) addPrefixToSingleKeyRESP(data []byte, args []string, prefix string, keyIndex int) []byte {
	if len(args) <= keyIndex {
		return data
	}

	originalKey := args[keyIndex]
	prefixedKey := prefix + originalKey

	// Rebuild the RESP array with the prefixed key
	return p.rebuildRESPArrayWithPrefix(data, args, keyIndex, prefixedKey)
}

// addPrefixToMultipleKeysRESP adds prefix to multiple keys starting from the specified position using RESP parsing
func (p *RedisProxy) addPrefixToMultipleKeysRESP(data []byte, args []string, prefix string, startIndex int) []byte {
	if len(args) <= startIndex {
		return data
	}

	// Create a copy of args with prefixed keys
	newArgs := make([]string, len(args))
	copy(newArgs, args)

	// Add prefix to all keys starting from startIndex
	for i := startIndex; i < len(newArgs); i++ {
		newArgs[i] = prefix + newArgs[i]
	}

	// Rebuild the RESP array
	return p.rebuildRESPArray(data, newArgs)
}

// addPrefixToEvalKeysRESP handles EVAL/EVALSHA commands which have a specific format using RESP parsing
func (p *RedisProxy) addPrefixToEvalKeysRESP(data []byte, args []string, prefix string) []byte {
	if len(args) < 3 {
		return data
	}

	// EVAL format: EVAL script numkeys key1 key2 ... arg1 arg2 ...
	// Parse numkeys
	numKeys, err := strconv.Atoi(args[2])
	if err != nil || numKeys <= 0 {
		return data
	}

	// Create a copy of args with prefixed keys
	newArgs := make([]string, len(args))
	copy(newArgs, args)

	// Add prefix to the specified number of keys (starting from position 3)
	for i := 3; i < 3+numKeys && i < len(newArgs); i++ {
		newArgs[i] = prefix + newArgs[i]
	}

	// Rebuild the RESP array
	return p.rebuildRESPArray(data, newArgs)
}

// rebuildRESPArray rebuilds a RESP array from the original data and new arguments
func (p *RedisProxy) rebuildRESPArray(data []byte, args []string) []byte {
	var result bytes.Buffer

	// Write array header
	result.WriteString(fmt.Sprintf("*%d\r\n", len(args)))

	// Write each argument as a bulk string
	for _, arg := range args {
		result.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg))
	}

	return result.Bytes()
}

// rebuildRESPArrayWithPrefix rebuilds a RESP array with a single prefixed key
func (p *RedisProxy) rebuildRESPArrayWithPrefix(data []byte, args []string, keyIndex int, prefixedKey string) []byte {
	// Create a copy of args with the prefixed key
	newArgs := make([]string, len(args))
	copy(newArgs, args)
	newArgs[keyIndex] = prefixedKey

	// Rebuild the RESP array
	return p.rebuildRESPArray(data, newArgs)
}

// filterScanResponse filters the keys in a SCAN response to only include those with the given prefix (nested array aware)
func (p *RedisProxy) filterScanResponse(data []byte, prefix string) []byte {
	val, _, err := p.parseRESP(data)
	if err != nil {
		return data
	}
	arr, ok := val.([]interface{})
	if !ok || len(arr) != 2 {
		return data
	}
	// arr[0] = cursor (string), arr[1] = keys ([]interface{})
	cursor, ok1 := arr[0].(string)
	keys, ok2 := arr[1].([]interface{})
	if !ok1 || !ok2 {
		return data
	}
	filtered := make([]interface{}, 0, len(keys))
	for _, k := range keys {
		if ks, ok := k.(string); ok && strings.HasPrefix(ks, prefix) {
			filtered = append(filtered, ks)
		}
	}
	newArr := []interface{}{cursor, filtered}
	return p.buildRESPArray(newArr)
}

func main() {
	// Configuration
	proxyAddr := getEnv("REDIS_PROXY_ADDR", ":6378")
	targetAddr :=  "127.0.0.1:6379"
	log.Printf("targetAddr: %s", targetAddr)
	// Create and start the proxy
	proxy := NewRedisProxy(proxyAddr, targetAddr)

	log.Printf("Starting Redis proxy")
	if err := proxy.Start(); err != nil {
		log.Fatalf("Failed to start proxy: %v", err)
	}
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
