#!/bin/sh
# Set up the data directory and register the service. Runs on install and on
# upgrade, so everything here must be idempotent.
set -e

# The SQLite database lives here. 0750 keeps it private to the refapp user and group.
mkdir -p /var/lib/refapp
chown refapp:refapp /var/lib/refapp
chmod 0750 /var/lib/refapp

# The config may hold secrets (e.g. DB.Postgres.Password) — readable by root and
# the refapp group only.
chown root:refapp /etc/refapp
chmod 0750 /etc/refapp
if [ -f /etc/refapp/config.yaml ]; then
    chown root:refapp /etc/refapp/config.yaml
    chmod 0640 /etc/refapp/config.yaml
fi

if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
    if [ "$1" = "configure" ] && [ -z "$2" ]; then
        # First install: enable and start, per Debian convention.
        systemctl enable --now refapp.service || true
    elif systemctl is-active --quiet refapp.service; then
        # Upgrade: respect the admin's enable/disable choice, restart into the
        # new binary only if it was already running.
        systemctl restart refapp.service || true
    fi
fi
