#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="${HOME}/.local/go/bin:${PATH}"
cd "$root"
mkdir -p dist
go test ./...
go build -o dist/gpa ./cmd/gpa
GOOS=windows GOARCH=amd64 go build -o dist/gpa.exe ./cmd/gpa
cp -f scripts/install.ps1 dist/install.ps1
rm -f dist/gpa-windows.zip
(
  cd dist
  zip -q gpa-windows.zip gpa.exe install.ps1
)
echo "Windows zip: $root/dist/gpa-windows.zip"
