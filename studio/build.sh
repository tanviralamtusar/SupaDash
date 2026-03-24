#!/bin/bash
# SupaDash Studio Builder
# Downloads official Supabase Studio, applies patches, and builds a custom Docker image.
#
# Usage: ./build.sh [STUDIO_VERSION] [IMAGE_TAG]
# Example: ./build.sh v2026.03.04 supadash/studio:latest

set -euo pipefail

STUDIO_VERSION="${1:-v2026.03.04}"
IMAGE_TAG="${2:-supadash/studio:latest}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="$SCRIPT_DIR/.build"
STUDIO_REPO="https://github.com/supabase/supabase.git"

echo "========================================"
echo "  SupaDash Studio Builder"
echo "  Version: $STUDIO_VERSION"
echo "  Image:   $IMAGE_TAG"
echo "========================================"

# Step 1: Clean previous build
echo ""
echo "[1/6] Cleaning previous build..."
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# Step 2: Clone Studio source (sparse checkout — only apps/studio)
echo "[2/6] Downloading Supabase Studio $STUDIO_VERSION..."
cd "$BUILD_DIR"
git clone --depth 1 --branch "$STUDIO_VERSION" --filter=blob:none --sparse "$STUDIO_REPO" supabase-src 2>/dev/null || {
    echo "⚠ Tag $STUDIO_VERSION not found, trying as branch..."
    git clone --depth 1 --filter=blob:none --sparse "$STUDIO_REPO" supabase-src
}
cd supabase-src
git sparse-checkout set apps/studio packages
echo "  ✓ Studio source downloaded"

# Step 3: Copy Studio app to build context
echo "[3/6] Extracting Studio app..."
cp -r apps/studio "$BUILD_DIR/studio"
cp -r packages "$BUILD_DIR/packages" 2>/dev/null || true
echo "  ✓ Studio app extracted"

# Step 4: Apply patches
echo "[4/6] Applying SupaDash patches..."
cd "$BUILD_DIR/studio"
"$SCRIPT_DIR/patch.sh" "$BUILD_DIR/studio" "$SCRIPT_DIR"
echo "  ✓ All patches applied"

# Step 5: Copy replacement files
echo "[5/6] Copying replacement files..."
if [ -d "$SCRIPT_DIR/files" ]; then
    cp -r "$SCRIPT_DIR/files/"* "$BUILD_DIR/studio/" 2>/dev/null || true
    echo "  ✓ Replacement files copied"
else
    echo "  (no replacement files)"
fi

# Step 6: Build Docker image
echo "[6/6] Building Docker image: $IMAGE_TAG..."
cp "$SCRIPT_DIR/Dockerfile" "$BUILD_DIR/studio/Dockerfile"
cd "$BUILD_DIR/studio"
docker build -t "$IMAGE_TAG" .
echo ""
echo "========================================"
echo "  ✅ Build complete!"
echo "  Image: $IMAGE_TAG"
echo "  Run:   docker run -p 3000:3000 $IMAGE_TAG"
echo "========================================"
