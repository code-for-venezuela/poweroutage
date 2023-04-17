#!/bin/bash

modprobe i2c-dev

export DBUS_SYSTEM_BUS_ADDRESS=unix:path=/host/run/dbus/system_bus_socket

# Choose a condition for running WiFi Connect according to your use case:

# Is there Internet connectivity via a google ping?
wget --spider http://google.com 2>&1

if [ $? -eq 0 ]; then
    printf 'Skipping WiFi Connect\n'
else
    printf 'Starting WiFi Connect\n'
    ./wifi-connect
fi

./poweroutage