#!/bin/sh
# Renders assets/openapi/openapi.yaml from its __TOKEN__ placeholders.
# Used for local (non-Docker) runs; the Docker build bakes the same values via
# the Dockerfile sed step. Usage: ./scripts/render-openapi.sh
set -e

DIR="$(cd "$(dirname "$0")/.." && pwd)"
FILE="$DIR/assets/openapi/openapi.yaml"

VERSION="${VERSION:-unknown}"
BUILD_DATE="${BUILD_DATE:-unknown}"

TMP="$FILE.tmp"
sed -e "s|__VERSION__|${VERSION}|g" \
    -e "s|__BUILD_DATE__|${BUILD_DATE}|g" \
    "$FILE" > "$TMP"
mv "$TMP" "$FILE"

echo "rendered $FILE (version=$VERSION build_date=$BUILD_DATE)"