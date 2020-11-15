#!/usr/bin/env bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

docker build $DIR/trapper/ -t trapper:exec  -f $DIR/trapper/Dockerfile.exec
docker build $DIR/trapper/ -t trapper:shell -f $DIR/trapper/Dockerfile.shell

docker rm --force trapper-exec  >/dev/null 2>&1
docker rm --force trapper-shell >/dev/null 2>&1

echo ""

docker run -d --name trapper-exec  --rm trapper:exec  >/dev/null
docker run -d --name trapper-shell --rm trapper:shell >/dev/null

docker ps
