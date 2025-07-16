package main

import (
	"bytes"
	"fmt"
	"testing"
)

func TestRESPParsing(t *testing.T) {
	// Sample AUTH command from user
	sampleCommand := "*3\r\n$4\r\nauth\r\n$6\r\nlukluk\r\n$6\r\n123123\r\n"

	// Create a proxy instance for testing
	proxy := &RedisProxy{}

	// Test parseRESPArray
	args, err := proxy.parseRESPArray([]byte(sampleCommand))
	if err != nil {
		t.Fatalf("Error parsing RESP array: %v", err)
	}

	expectedArgs := []string{"auth", "lukluk", "123123"}
	if len(args) != len(expectedArgs) {
		t.Fatalf("Expected %d arguments, got %d", len(expectedArgs), len(args))
	}

	for i, expected := range expectedArgs {
		if args[i] != expected {
			t.Errorf("Expected arg[%d] = %q, got %q", i, expected, args[i])
		}
	}

	// Test isAuthCommand
	isAuth := proxy.isAuthCommand([]byte(sampleCommand))
	if !isAuth {
		t.Error("Expected isAuthCommand to return true")
	}

	// Test extractAuthUsername
	username := proxy.extractAuthUsername([]byte(sampleCommand))
	if username != "lukluk" {
		t.Errorf("Expected username 'lukluk', got %q", username)
	}

	// Test extractAuthPassword
	password := proxy.extractAuthPassword([]byte(sampleCommand))
	if password != "123123" {
		t.Errorf("Expected password '123123', got %q", password)
	}

	// Test rebuilding the RESP array
	rebuilt := proxy.rebuildRESPArray([]byte(sampleCommand), args)
	if !bytes.Equal(rebuilt, []byte(sampleCommand)) {
		t.Errorf("Rebuilt command doesn't match original:\nOriginal: %q\nRebuilt:  %q", sampleCommand, rebuilt)
	}

	fmt.Println("All tests passed!")
}

func TestRESPParsingWithPrefix(t *testing.T) {
	// Test with a simple SET command
	setCommand := "*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n"

	proxy := &RedisProxy{}

	args, err := proxy.parseRESPArray([]byte(setCommand))
	if err != nil {
		t.Fatalf("Error parsing SET command: %v", err)
	}

	expectedArgs := []string{"SET", "key", "value"}
	for i, expected := range expectedArgs {
		if args[i] != expected {
			t.Errorf("Expected arg[%d] = %q, got %q", i, expected, args[i])
		}
	}

	// Test adding prefix to key
	prefixedArgs := make([]string, len(args))
	copy(prefixedArgs, args)
	prefixedArgs[1] = "testprefix:" + prefixedArgs[1] // Add prefix to key

	prefixed := proxy.rebuildRESPArray([]byte(setCommand), prefixedArgs)
	expectedPrefixed := "*3\r\n$3\r\nSET\r\n$14\r\ntestprefix:key\r\n$5\r\nvalue\r\n"

	if !bytes.Equal(prefixed, []byte(expectedPrefixed)) {
		t.Errorf("Prefixed command doesn't match expected:\nExpected: %q\nGot:      %q", expectedPrefixed, prefixed)
	}

	fmt.Println("Prefix tests passed!")
}
