#!/usr/bin/env bash
# Verifies that the Go and TypeScript codecs agree.
#
# Run this before deploying: a divergence here means the browser would encode
# questionnaire data that the server cannot decode, which presents as garbage
# rather than as an error at the point of the bug.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "==> regenerating artifacts from the Go implementation"
(cd backend && go run ./cmd/genvectors -skip-bitmap >/dev/null)

echo "==> checking that the checked-in vectors are current"
if ! git diff --quiet -- spec/vectors backend/internal/codec/spec_pins.go backend/internal/codec/tables/validity.bin 2>/dev/null; then
  echo "ERROR: codec artifacts are stale. Run 'make vectors' and commit the result." >&2
  git --no-pager diff --stat -- spec/vectors backend/internal/codec >&2
  exit 1
fi
echo "    in sync"

echo "==> Go test suite"
(cd backend && go test ./... -count=1)

echo "==> TypeScript test suite (includes bidirectional cross-execution)"
(cd frontend && npx vitest run)

echo
echo "conformance OK"
