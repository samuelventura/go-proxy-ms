# go-proxy-ms

## API

```bash
#ship management
curl -X GET http://127.0.0.1:31688/api/ship/count
curl -X GET http://127.0.0.1:31688/api/ship/count/enabled
curl -X GET http://127.0.0.1:31688/api/ship/count/disabled
curl -X GET http://127.0.0.1:31688/api/ship/info/:name
curl -X POST http://127.0.0.1:31688/api/ship/add/:name?prefix=http://host:port/path
curl -X POST http://127.0.0.1:31688/api/ship/remove/:name
curl -X POST http://127.0.0.1:31688/api/ship/enable/:name
curl -X POST http://127.0.0.1:31688/api/ship/disable/:name
```

## Test Drive

```bash
ln -sf ~/github/go-proxy-ms/server.crt ~/go/bin/go-proxy-ms.crt
ln -sf ~/github/go-proxy-ms/server.key ~/go/bin/go-proxy-ms.key
go install && ~/go/bin/go-proxy-ms
tail /usr/local/bin/go-proxy-ms.out.log -n 10
#as daemon
curl -X POST http://127.0.0.1:31600/api/daemon/env/proxy \
     -H "DaemonEnviron: PROXY_SERVER_CRT=$HOME/src/go-proxy-ms/server.crt" \
     -H "DaemonEnviron: PROXY_SERVER_KEY=$HOME/src/go-proxy-ms/server.key" 
#ship management
curl -X GET http://127.0.0.1:31688/api/ship/count
curl -X GET http://127.0.0.1:31688/api/ship/count/enabled
curl -X GET http://127.0.0.1:31688/api/ship/count/disabled
curl -X GET http://127.0.0.1:31688/api/ship/info/demo
curl -X POST http://127.0.0.1:31688/api/ship/add/demo?prefix=http://127.0.0.1:80
curl -X POST http://127.0.0.1:31688/api/ship/remove/demo
curl -X POST http://127.0.0.1:31688/api/ship/enable/demo
curl -X POST http://127.0.0.1:31688/api/ship/disable/demo
```

## Test Certificates

```bash
openssl genrsa -out server.key 2048
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 36500
Country Name (2 letter code) [AU]:MX
State or Province Name (full name) [Some-State]:SLP
Locality Name (eg, city) []:San Luis Potosi
Organization Name (eg, company) [Internet Widgits Pty Ltd]:Samuel Ventura
Organizational Unit Name (eg, section) []:
Common Name (e.g. server FQDN or YOUR name) []:
Email Address []:
```
