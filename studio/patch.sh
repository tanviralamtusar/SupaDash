#!/bin/bash
# SupaDash Patch Applier
# Applies all patch files from the patches/ directory to the Studio source.
#
# Usage: ./patch.sh <STUDIO_DIR> <SCRIPT_DIR>

set -euo pipefail

STUDIO_DIR="${1:-.}"
SCRIPT_DIR="${2:-.}"
PATCHES_DIR="$SCRIPT_DIR/patches"

if [ ! -d "$PATCHES_DIR" ]; then
    echo "  No patches directory found at $PATCHES_DIR"
    exit 0
fi

# Apply patches in order (sorted by filename)
PATCH_COUNT=0
FAIL_COUNT=0

for patch_file in "$PATCHES_DIR"/*.patch; do
    [ -f "$patch_file" ] || continue
    
    patch_name=$(basename "$patch_file")
    echo "  Applying: $patch_name"
    
    # Try to apply patch; if it fails, try with --fuzz=3
    if patch -d "$STUDIO_DIR" -p1 --forward --batch < "$patch_file" > /dev/null 2>&1; then
        echo "    ✓ Applied cleanly"
        PATCH_COUNT=$((PATCH_COUNT + 1))
    elif patch -d "$STUDIO_DIR" -p1 --forward --batch --fuzz=3 < "$patch_file" > /dev/null 2>&1; then
        echo "    ⚠ Applied with fuzz"
        PATCH_COUNT=$((PATCH_COUNT + 1))
    else
        echo "    ✗ FAILED — may need manual update for new Studio version"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
done

echo ""
echo "  Patches applied: $PATCH_COUNT, Failed: $FAIL_COUNT"

if [ $FAIL_COUNT -gt 0 ]; then
    echo "  ⚠ Some patches failed! Review manually before building."
    exit 1
fi
