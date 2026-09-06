#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="${HOME}/.local/go/bin:${PATH}"
cd "$root"
mkdir -p dist
go test ./...
go build -o dist/gpa ./cmd/gpa
GOOS=windows GOARCH=amd64 go build -o dist/gpa.exe ./cmd/gpa
./dist/gpa install --linux-bin dist/gpa --windows-bin dist/gpa.exe
