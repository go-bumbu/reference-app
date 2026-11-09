#!/bin/sh
# Purge removes configuration and the system user; plain remove keeps both, and
# /var/lib/refapp (the database) is never deleted automatically — the admin can
# drop it by hand.
set -e

if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
fi

if [ "$1" = "purge" ]; then
    rm -f /etc/refapp/config.yaml
    rmdir --ignore-fail-on-non-empty /etc/refapp 2>/dev/null || true

    if getent passwd refapp >/dev/null; then
        deluser --system refapp || true
    fi
    if getent group refapp >/dev/null; then
        delgroup --system refapp || true
    fi

    echo "refapp: /var/lib/refapp was kept; remove it manually to delete the database."
fi
