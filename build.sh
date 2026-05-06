#!/bin/bash

set -e

echo "========== Building Moonlight Registry =========="

echo "[1/4] Building frontend..."
cd web
npm install
npm run build
cd ..

echo "[2/4] Preparing embedded frontend..."
rm -rf cmd/registry/dist
cp -r web/dist cmd/registry/dist

echo "[3/4] Building backend with embedded frontend..."
go build -o registry ./cmd/registry

echo "[4/4] Cleaning up temporary files..."
rm -rf cmd/registry/dist

echo "========== Build Complete =========="
echo "Binary: ./registry (includes embedded frontend)"
echo ""
echo "Run: ./registry --config configs/config.yaml"
echo "Note: The binary now contains the frontend, no web/dist needed at runtime!"
