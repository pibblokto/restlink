#!/bin/bash
set -e

if [ -z "$1" ]; then
    echo "Error: Tag argument is missing"
    exit 1
fi

TAG="$1"

docker build -t piblokto/restlink:"${TAG}" .
docker push piblokto/restlink:"${TAG}"