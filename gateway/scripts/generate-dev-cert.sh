#!/bin/sh
set -eu

if [ ! -f docker-compose.yml ]; then
	echo "run this script from the repository root" >&2
	exit 1
fi

mkdir -p gateway/.local/certs
openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 30 -keyout gateway/.local/certs/server.key -out gateway/.local/certs/server.crt -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,DNS:users.internal.test,DNS:orders.internal.test,DNS:products.internal.test,IP:127.0.0.1"
chmod 0644 gateway/.local/certs/server.crt gateway/.local/certs/server.key
