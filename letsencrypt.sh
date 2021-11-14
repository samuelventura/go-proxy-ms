#!/bin/bash -x

V2="--server https://acme-v02.api.letsencrypt.org/directory"

#sudo apt install letsencrypt
curl -X POST http://127.0.0.1:31600/api/daemon/stop/$PROXY_DAEMON
sudo letsencrypt certonly --standalone -d $PROXY_HOSTNAME --force-renew $V2
curl -X POST http://127.0.0.1:31600/api/daemon/start/$PROXY_DAEMON
