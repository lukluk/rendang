#!/bin/bash

# Redis Proxy Service Uninstallation Script
# This script removes the Redis proxy systemd service

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

echo -e "${YELLOW}Redis Proxy Service Uninstallation${NC}"
echo "====================================="

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo -e "${RED}Error: This script must be run as root${NC}"
   exit 1
fi

# Stop and disable the service
echo -e "${YELLOW}Stopping and disabling service...${NC}"
if systemctl is-active --quiet $SERVICE_NAME; then
    systemctl stop $SERVICE_NAME
    echo "Service stopped"
else
    echo "Service was not running"
fi

if systemctl is-enabled --quiet $SERVICE_NAME; then
    systemctl disable $SERVICE_NAME
    echo "Service disabled"
else
    echo "Service was not enabled"
fi

# Remove service file
echo -e "${YELLOW}Removing service file...${NC}"
if [ -f "$SERVICE_FILE" ]; then
    rm -f $SERVICE_FILE
    echo "Removed service file: $SERVICE_FILE"
else
    echo "Service file not found: $SERVICE_FILE"
fi

# Reload systemd
systemctl daemon-reload

# Remove installation directory
echo -e "${YELLOW}Removing installation directory...${NC}"
if [ -d "$INSTALL_DIR" ]; then
    rm -rf $INSTALL_DIR
    echo "Removed installation directory: $INSTALL_DIR"
else
    echo "Installation directory not found: $INSTALL_DIR"
fi

# Remove log directory (optional - ask user)
echo -e "${YELLOW}Log directory cleanup...${NC}"
if [ -d "$LOG_DIR" ]; then
    read -p "Remove log directory $LOG_DIR? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf $LOG_DIR
        echo "Removed log directory: $LOG_DIR"
    else
        echo "Log directory preserved: $LOG_DIR"
    fi
else
    echo "Log directory not found: $LOG_DIR"
fi

# Remove service user and group (optional - ask user)
echo -e "${YELLOW}User cleanup...${NC}"
if getent passwd $SERVICE_USER > /dev/null 2>&1; then
    read -p "Remove service user '$SERVICE_USER'? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        userdel $SERVICE_USER
        echo "Removed user: $SERVICE_USER"
    else
        echo "User preserved: $SERVICE_USER"
    fi
else
    echo "Service user not found: $SERVICE_USER"
fi

if getent group $SERVICE_GROUP > /dev/null 2>&1; then
    read -p "Remove service group '$SERVICE_GROUP'? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        groupdel $SERVICE_GROUP
        echo "Removed group: $SERVICE_GROUP"
    else
        echo "Group preserved: $SERVICE_GROUP"
    fi
else
    echo "Service group not found: $SERVICE_GROUP"
fi

echo -e "${GREEN}✅ Redis proxy service uninstalled successfully!${NC}" 