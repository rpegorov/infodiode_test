
#!/bin/bash

# Script to initialize volume directories with proper permissions
# This script should be run before starting docker-compose

set -e

echo "Initializing volume directories..."

# Create log directories
mkdir -p logs/sender logs/recipient
mkdir -p mosquitto/data mosquitto/log
mkdir -p prometheus grafana/provisioning/sender grafana/provisioning/recipient
mkdir -p postgres
mkdir -p data

# Set permissions for log directories
# UID 1000 is what we use in Dockerfiles for sender/recipient users
echo "Setting permissions for log directories..."
chmod -R 755 logs
# Try to set ownership if running as root
if [ "$EUID" -eq 0 ]; then
    chown -R 1000:1000 logs/sender logs/recipient
    echo "Ownership set to UID/GID 1000"
else
    echo "Note: Not running as root, cannot change ownership."
    echo "Container user (UID 1000) may not have write access."
    echo "You may need to run: sudo chown -R 1000:1000 logs/"
fi

# Set permissions for mosquitto directories
chmod -R 755 mosquitto

# Set permissions for other directories
chmod -R 755 prometheus
chmod -R 755 grafana
chmod -R 755 postgres
chmod -R 755 data

echo "Volume directories initialized."

# Check if we can write to log directories
if touch logs/sender/test 2>/dev/null; then
    rm logs/sender/test
    echo "✓ logs/sender is writable"
else
    echo "⚠ Warning: logs/sender is not writable by current user"
fi

if touch logs/recipient/test 2>/dev/null; then
    rm logs/recipient/test
    echo "✓ logs/recipient is writable"
else
    echo "⚠ Warning: logs/recipient is not writable by current user"
fi

echo ""
echo "If you still get permission errors, try one of these:"
echo "1. Run: sudo chown -R 1000:1000 logs/"
echo "2. Run: sudo chmod -R 777 logs/ (less secure)"
echo "3. Use named Docker volumes instead of bind mounts"
