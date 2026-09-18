#!/usr/bin/env bash
# The adapter is a separate module. Inside go.work it always resolves the root
# from the tree, so a require on a version that was never published builds fine
# here and fails for the first consumer who types `go get` — which is what
# happened to adapters/gin v0.1.0's require on an unreleased apikit v0.6.0.
#
# This check leaves the workspace and an empty module cache, so it sees what a
# consumer sees.
#
#   ./scripts/check-adapter-module.sh             run the checks
#   ./scripts/check-adapter-module.sh --selftest  force each red arm, then run them
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT=$PWD
ADAPTER=adapters/gin
MODULE=github.com/open-rails/apikit

fail() { echo "FAIL: $*" >&2; exit 1; }
# The module cache is written read-only; make it removable again.
rmcache() { [ -n "${1:-}" ] && chmod -R u+w "$1" 2>/dev/null; rm -rf "${1:?}"; }

# 1. No committed replace. A bootstrap replace makes every other check lie.
check_no_replace() {
  local modfile=${1:-$ROOT/$ADAPTER/go.mod}
  if grep -qE '^[[:space:]]*replace|^replace[[:space:]]*\(' "$modfile"; then
    echo "the adapter go.mod still carries a replace directive:" >&2
    grep -nE 'replace' "$modfile" >&2
    return 1
  fi
}

# 2. The pinned root version must exist on the module proxy.
check_pin_published() {
  local modfile=${1:-$ROOT/$ADAPTER/go.mod}
  local pin
  pin=$(awk -v m="$MODULE" '$1 == m { print $2 }' "$modfile" | head -1)
  [ -n "$pin" ] || { echo "the adapter does not require $MODULE" >&2; return 1; }
  local cache
  cache=$(mktemp -d)
  trap 'rmcache "$cache"' RETURN
  if ! GOWORK=off GOFLAGS= GOMODCACHE="$cache" GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" \
       go mod download "$MODULE@$pin" >/dev/null 2>&1; then
    echo "the adapter requires $MODULE@$pin, which the proxy does not serve" >&2
    return 1
  fi
  echo "  pinned root $pin is published"
}

# 3. The PUBLISHED adapter must build for a consumer from an empty cache. This
#    is the exact path that was broken; it does not depend on the working tree.
check_published_adapter_builds() {
  local tag
  tag=$(git tag -l "$ADAPTER/v*" | sort -V | tail -1)
  if [ -z "$tag" ]; then
    echo "  no published adapter tag yet — 0 published adapters checked"
    return 0
  fi
  local version=${tag#"$ADAPTER/"}
  local dir cache
  dir=$(mktemp -d)
  cache=$(mktemp -d)
  trap 'rm -rf "$dir"; rmcache "$cache"' RETURN
  cat > "$dir/go.mod" <<'GOMOD'
module consumer

go 1.23.0
GOMOD
  cat > "$dir/main.go" <<'MAIN'
package main

import (
	"github.com/open-rails/apikit"
	apikitgin "github.com/open-rails/apikit/adapters/gin"
)

func main() { _, _ = apikitgin.Fail, apikit.TypeForStatus }
MAIN
  if ! (cd "$dir" && GOWORK=off GOFLAGS= GOMODCACHE="$cache" \
        GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" \
        go get "$MODULE/$ADAPTER@$version" >/dev/null 2>&1 && \
        GOWORK=off GOFLAGS= GOMODCACHE="$cache" go build ./... >/dev/null 2>&1); then
    echo "a consumer cannot build $MODULE/$ADAPTER@$version from a clean cache" >&2
    return 1
  fi
  echo "  published $ADAPTER@$version builds for a clean consumer"
}

# 4. The adapter IN THIS TREE must build and test against the root IN THIS TREE
#    without go.work — the workspace is a convenience, not a contract.
check_tree_adapter_builds() {
  local dir
  dir=$(mktemp -d)
  trap 'rm -rf "$dir"' RETURN
  mkdir -p "$dir/tree"
  tar -C "$ROOT" --exclude=.git --exclude=go.work --exclude=go.work.sum -cf - . | tar -C "$dir/tree" -xf -
  (cd "$dir/tree/$ADAPTER" && GOWORK=off go mod edit -replace "$MODULE=../.." &&
    GOWORK=off go build ./... && GOWORK=off go test ./... >/dev/null) ||
    { echo "the in-tree adapter does not build against the in-tree root" >&2; return 1; }
  echo "  in-tree adapter builds and tests against the in-tree root"
}

selftest() {
  local dir; dir=$(mktemp -d); trap 'rm -rf "$dir"' RETURN
  printf 'module x\n\ngo 1.23.0\n\nrequire %s v0.6.0\n\nreplace %s => ../..\n' "$MODULE" "$MODULE" > "$dir/replace.mod"
  check_no_replace "$dir/replace.mod" 2>/dev/null && fail "selftest: a committed replace was not caught"
  printf 'module x\n\ngo 1.23.0\n\nrequire %s v99.99.99\n' "$MODULE" > "$dir/unpublished.mod"
  check_pin_published "$dir/unpublished.mod" 2>/dev/null && fail "selftest: an unpublished pin was not caught"
  echo "selftest: both red arms fire"
}

[ "${1:-}" = "--selftest" ] && selftest

echo "adapter module independence:"
check_no_replace || fail "adapters/gin/go.mod carries a replace"
check_pin_published || fail "the adapter's root pin is not published"
check_published_adapter_builds || fail "the published adapter does not resolve"
check_tree_adapter_builds || fail "the in-tree adapter does not build standalone"
echo "OK"
