package main

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

func debugMain() {
	fmt.Println("Starting debug Go client...")

	// Create Redis client pointing to the proxy
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6378", // Proxy address
		Password: "",               // No password for proxy
		DB:       0,
	})

	ctx := context.Background()

	// Test connection
	fmt.Println("Testing connection...")
	_, err := client.Ping(ctx).Result()
	if err != nil {
		log.Printf("Failed to connect to Redis proxy: %v", err)
		return
	}
	fmt.Println("Connected to Redis proxy successfully")

	// Test AUTH with username and password
	fmt.Println("Testing AUTH...")
	err = client.Do(ctx, "AUTH", "testuser", "testpass").Err()
	if err != nil {
		log.Printf("AUTH failed: %v", err)
	} else {
		fmt.Println("AUTH successful")
	}

	// Test SET command
	fmt.Println("Testing SET...")
	err = client.Set(ctx, "mykey", "myvalue", 0).Err()
	if err != nil {
		log.Printf("SET failed: %v", err)
	} else {
		fmt.Println("SET command successful")
	}

	// Test GET command
	fmt.Println("Testing GET...")
	val, err := client.Get(ctx, "mykey").Result()
	if err != nil {
		log.Printf("GET failed: %v", err)
	} else {
		fmt.Printf("GET result: %s\n", val)
	}

	fmt.Println("Debug Go client test completed")
}
