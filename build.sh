#! /bin/bash

rm -rf bin
mkdir -p bin

go test ./...
go build -o bin/git-pr