#!/bin/bash
# Wrapper for fcp-terraform — provides install instructions if binary is missing.
if command -v fcp-terraform &>/dev/null; then
  exec fcp-terraform "$@"
else
  echo "fcp-terraform not found. Install:" >&2
  echo "  curl -LsSf https://github.com/os-tack/fcp-terraform/releases/latest/download/install.sh | sh" >&2
  echo "" >&2
  echo "Or build from source:" >&2
  echo "  go install github.com/os-tack/fcp-terraform/cmd/fcp-terraform@latest" >&2
  exit 1
fi
