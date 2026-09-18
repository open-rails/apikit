#!/usr/bin/env bash
# Re-extracts every vendored parser from its source checkout and diffs. Reports
# the COUNT it actually compared: an absent checkout is named and refused, never
# quietly skipped, because a scan that cannot run must not look like a pass.
set -euo pipefail
cd "$(dirname "$0")"

DOUJINS=${DOUJINS_REPO:-$HOME/doujins}
HENTAI0=${HENTAI0_REPO:-$HOME/hentai0}

declare -a PAIRS=(
  "$DOUJINS/frontend/src/lib/http/client.ts|vendor/doujins/lib/http/client.ts"
  "$HENTAI0/frontend/src/services/api/api-service.ts|vendor/hentai0/services/api/api-service.ts"
  "$HENTAI0/frontend/src/hooks/upload/useUploadVideo.ts|vendor/hentai0/hooks/upload/useUploadVideo.ts"
  "$HENTAI0/frontend/src/types/api/errors.ts|vendor/hentai0/types/api/errors.ts"
)

checked=0 failed=0
for pair in "${PAIRS[@]}"; do
  src=${pair%%|*}
  dst=${pair##*|}
  if [ ! -f "$src" ]; then
    echo "REFUSED: source checkout missing: $src" >&2
    failed=$((failed + 1))
    continue
  fi
  if ! diff -u "$dst" "$src" >/dev/null; then
    echo "DRIFT: $dst no longer matches $src" >&2
    diff -u "$dst" "$src" || true
    failed=$((failed + 1))
    continue
  fi
  checked=$((checked + 1))
done

echo "verified $checked/${#PAIRS[@]} vendored parsers"
if [ "$checked" -ne "${#PAIRS[@]}" ] || [ "$failed" -ne 0 ]; then
  echo "vendored parsers are not provably current" >&2
  exit 1
fi
