#!/usr/bin/env bash
# Validate that every GitHub Action `uses:` reference in .github/workflows
# is pinned to a full 40-character commit SHA, not a mutable tag or branch.
# Reusable workflows (`owner/repo/.github/workflows/x.yml@ref`) and local
# actions (`./path`) are allowed to use tags/relative refs.
set -euo pipefail

WORKFLOW_DIR=".github/workflows"
fail=0

if [ ! -d "$WORKFLOW_DIR" ]; then
  echo "No $WORKFLOW_DIR directory; nothing to validate."
  exit 0
fi

while IFS= read -r line; do
  file="${line%%:*}"
  rest="${line#*:}"
  # Extract the ref after the last '@'.
  ref="$(printf '%s\n' "$rest" | sed -E 's/.*@([^ ]+).*/\1/')"
  uses="$(printf '%s\n' "$rest" | sed -E 's/.*uses:[[:space:]]*//; s/[[:space:]]*#.*//')"

  # Allow local actions (./...) — no ref to pin.
  case "$uses" in
    ./*) continue ;;
  esac

  # A valid pin is a 40-hex-char SHA.
  if printf '%s\n' "$ref" | grep -Eq '^[0-9a-f]{40}$'; then
    continue
  fi

  echo "ERROR: $file pins an action to a non-SHA ref: $uses" >&2
  fail=1
done < <(grep -rEn '^[[:space:]]*-?[[:space:]]*uses:[[:space:]]*[^.]' "$WORKFLOW_DIR")

if [ "$fail" -ne 0 ]; then
  echo "" >&2
  echo "Pin actions to a full commit SHA with a version comment, e.g.:" >&2
  echo "  uses: actions/checkout@<40-char-sha> # v6.0.3" >&2
  exit 1
fi

echo "All GitHub Action references are pinned to commit SHAs."
