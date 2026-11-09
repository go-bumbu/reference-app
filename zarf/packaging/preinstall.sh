#!/bin/sh
# Create the system user before any files land, so postinstall can chown to it.
set -e

if ! getent group refapp >/dev/null; then
    addgroup --system refapp
fi

if ! getent passwd refapp >/dev/null; then
    # The home dir must exist before adduser or it warns; postinstall fixes up
    # ownership and mode afterwards.
    mkdir -p /var/lib/refapp
    adduser --system --ingroup refapp --no-create-home \
        --home /var/lib/refapp --shell /usr/sbin/nologin \
        --gecos "refapp server" refapp
fi
