#!/bin/bash -x

#curl -X POST http://127.0.0.1:31600/api/daemon/stop/proxy
#curl -X POST http://127.0.0.1:31600/api/daemon/disable/proxy
#curl -X POST http://127.0.0.1:31600/api/daemon/uninstall/proxy
if [[ "$OSTYPE" == "linux"* ]]; then
    DAEMON=proxy
    BINARY=go-$DAEMON-ms
    SRC=$HOME/go/bin
    DST=/usr/local/bin
    go install
    curl -X POST http://127.0.0.1:31600/api/daemon/stop/$DAEMON
    curl -X POST http://127.0.0.1:31600/api/daemon/uninstall/$DAEMON
    sudo cp $SRC/$BINARY $DST
    curl -X POST http://127.0.0.1:31600/api/daemon/install/$DAEMON?path=$DST/$BINARY
    curl -X POST http://127.0.0.1:31600/api/daemon/enable/$DAEMON
    curl -X POST http://127.0.0.1:31600/api/daemon/start/$DAEMON
    curl -X GET http://127.0.0.1:31600/api/daemon/info/$DAEMON
    curl -X GET http://127.0.0.1:31600/api/daemon/env/$DAEMON
fi