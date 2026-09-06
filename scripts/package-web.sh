#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="${HOME}/.local/go/bin:${PATH}"
cd "$root"
mkdir -p dist
go test ./...
go build -o dist/gpa ./cmd/gpa
go build -o dist/gpa-manager ./cmd/gpa-manager
GOOS=windows GOARCH=amd64 go build -o dist/gpa-manager.exe ./cmd/gpa-manager
GOOS=windows GOARCH=amd64 go build -o dist/gpa.exe ./cmd/gpa
cp -f agent/gpa_agent.py skills/gpa-account-manager/scripts/gpa_agent.py
if ! cmp -s agent/gpa_agent.py skills/gpa-account-manager/scripts/gpa_agent.py; then
  echo "skill script drifted from agent/gpa_agent.py" >&2
  exit 1
fi
cp -f scripts/install.ps1 dist/install.ps1
rm -f dist/gpa-windows.zip dist/gpa-manager-windows.zip
(
  cd dist
  cp -f gpa-manager.exe "GPA Manager.exe"
  zip -q gpa-manager-windows.zip "GPA Manager.exe"
  zip -q gpa-windows.zip gpa.exe install.ps1
)
echo "Windows manager zip: $root/dist/gpa-manager-windows.zip"
