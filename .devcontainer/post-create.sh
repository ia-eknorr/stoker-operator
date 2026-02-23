#!/usr/bin/env bash
set -euo pipefail

echo "==> Downloading Go modules..."
go mod download

echo "==> Configuring git hooks..."
make setup

echo "==> Installing Docusaurus dependencies..."
if [ -f docs/package-lock.json ]; then
  (cd docs && npm ci)
elif [ -f docs/package.json ]; then
  (cd docs && npm install)
fi

echo "==> Installing Claude Code..."
npm install -g @anthropic-ai/claude-code 2>/dev/null || echo "    (skipped — npm install failed, install manually if needed)"

echo ""
echo "==> Tool versions:"
go version
kubectl version --client --short 2>/dev/null || kubectl version --client
kind version
helm version --short
controller-gen --version
golangci-lint version --short 2>/dev/null || golangci-lint version
kustomize version
helm-docs --version 2>/dev/null || true
echo ""
echo "==> Ready! Create a kind cluster with: kind create cluster --name dev"
