#!/bin/bash

# Redis Proxy Service Installation Script
# This script installs the Redis proxy as a systemd service

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
SERVICE_NAME="redis-proxy"
SERVICE_USER="redis-proxy"
SERVICE_GROUP="redis-proxy"
INSTALL_DIR="/opt/redis-proxy"
SERVICE_FILE="/etc/systemd/system/redis-proxy.service"
LOG_DIR="/var/log/redis-proxy"

echo -e "${GREEN}Redis Proxy Service Installation${NC}"
echo "=================================="

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed. Please install Go first.${NC}"
    exit 1
fi

# Build the proxy
echo -e "${YELLOW}Building Redis proxy...${NC}"
if [ ! -f "go.mod" ]; then
    echo "Initializing Go module..."
    go mod init redis-proxy
fi

go build -o redis-proxy main.go

if [ $? -ne 0 ]; then
    echo -e "${RED}Failed to build Redis proxy${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Redis proxy built successfully!${NC}"
# If the service already exists, just restart it and exit


# Create service user and group
echo -e "${YELLOW}Creating service user and group...${NC}"
if ! sudo getent group $SERVICE_GROUP > /dev/null 2>&1; then
    sudo groupadd -r $SERVICE_GROUP
    echo "Created group: $SERVICE_GROUP"
else
    echo "Group $SERVICE_GROUP already exists"
fi

if ! sudo getent passwd $SERVICE_USER > /dev/null 2>&1; then
    sudo useradd -r -g $SERVICE_GROUP -s /bin/false -d $INSTALL_DIR $SERVICE_USER
    echo "Created user: $SERVICE_USER"
else
    echo "User $SERVICE_USER already exists"
fi

# Create installation directory
echo -e "${YELLOW}Creating installation directory...${NC}"
sudo mkdir -p $INSTALL_DIR
sudo mkdir -p $LOG_DIR

# Copy files
echo -e "${YELLOW}Installing Redis proxy...${NC}"
sudo cp redis-proxy $INSTALL_DIR/
sudo cp redis-proxy.service $SERVICE_FILE

# Replace REPLACE_IP in the service file with the actual local IP address
if grep -q "REPLACE_IP" "$SERVICE_FILE"; then
    LOCAL_IP=$(ip -4 addr show eth0 | grep -oP '(?<=inet\s)\d+(\.\d+){3}')
    sudo sed -i "s/REPLACE_IP/$LOCAL_IP/g" "$SERVICE_FILE"
    echo "Set LOCAL_IP=$LOCAL_IP in $SERVICE_FILE"
fi


# Set permissions
sudo chown -R $SERVICE_USER:$SERVICE_GROUP $INSTALL_DIR
sudo chown -R $SERVICE_USER:$SERVICE_GROUP $LOG_DIR
sudo chmod 755 $INSTALL_DIR/redis-proxy
sudo chmod 644 $SERVICE_FILE

# Reload systemd and enable service
echo -e "${YELLOW}Configuring systemd service...${NC}"
sudo systemctl daemon-reload
sudo systemctl enable $SERVICE_NAME

echo -e "${GREEN}✅ Redis proxy service installed successfully!${NC}"
echo ""
echo -e "${GREEN}Service Information:${NC}"
echo "  Service Name: $SERVICE_NAME"
echo "  Install Directory: $INSTALL_DIR"
echo "  Log Directory: $LOG_DIR"
echo ""
echo -e "${GREEN}Service Commands:${NC}"
echo "  Start service:     systemctl start $SERVICE_NAME"
echo "  Stop service:      systemctl stop $SERVICE_NAME"
echo "  Restart service:   systemctl restart $SERVICE_NAME"
echo "  Check status:      systemctl status $SERVICE_NAME"
echo "  View logs:         journalctl -u $SERVICE_NAME -f"
echo ""
echo -e "${GREEN}Configuration:${NC}"
echo "  After changing config, restart: systemctl restart $SERVICE_NAME"
echo ""
echo -e "${YELLOW}Note: The service is enabled but not started.${NC}"
echo -e "${YELLOW}Run 'systemctl start $SERVICE_NAME' to start it now.${NC}" 