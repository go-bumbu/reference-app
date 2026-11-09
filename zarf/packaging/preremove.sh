#!/bin/sh
# Stop the service before the binary goes away — but only on actual removal.
# On upgrade dpkg passes "upgrade"/"deconfigure" and the service is deliberately
# left running: replacing the binary file is safe on Linux (the running process
# keeps its inode), and postinstall restarts it into the new version. Stopping
# here instead would make postinstall's is-active check see a stopped service
# and leave it down after every upgrade.
set -e

if [ "$1" = "remove" ] && [ -d /run/systemd/system ]; then
    if systemctl is-active --quiet refapp.service; then
        systemctl stop refapp.service || true
    fi
    systemctl disable refapp.service || true
fi
