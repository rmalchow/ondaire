#!/bin/bash
cd $(dirname $0)
export home=$(pwd)
echo "home directory is: ${home}"
docker compose pull
./stop.sh || /usr/bin/true
docker compose up -d
