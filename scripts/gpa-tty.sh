#!/usr/bin/env bash
set -euo pipefail
export PATH="${HOME}/.local/bin:${PATH}"
cd /home/zheng/gpt-account
exec gpa "$@"
