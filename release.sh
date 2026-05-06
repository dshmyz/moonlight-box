#!/bin/bash

set -e

WORK_DIR=$(pwd)
DIST_DIR="$WORK_DIR/dist"
APP_NAME="moonlight-registry"

echo "========== Preparing Release Package =========="

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

echo "[1/3] Building (frontend embedded in binary)..."
bash build.sh

echo "[2/3] Copying binary..."
cp registry "$DIST_DIR/"

echo "[3/3] Copying configs..."
mkdir -p "$DIST_DIR/configs"
if [ -f "configs/config.yaml" ]; then
    cp configs/config.yaml "$DIST_DIR/configs/"
fi
cp -r configs/*.yaml.example "$DIST_DIR/configs/" 2>/dev/null || true

echo "========== Release Package Ready =========="
echo "Location: $DIST_DIR"
echo ""
echo "Deploy steps:"
echo "1. Upload dist/ to server"
echo "2. Configure configs/config.yaml"
echo "3. Run: ./registry --config configs/config.yaml"
echo ""
echo "Note: The binary contains the embedded frontend, only config is needed at runtime!"
