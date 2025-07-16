#!/bin/bash

echo "Testing Redis proxy AUTH command parsing..."

# Test the sample AUTH command
echo -e "*3\r\n\$4\r\nauth\r\n\$6\r\nlukluk\r\n\$6\r\n123123\r\n" | nc localhost 6378

echo "Test completed." 