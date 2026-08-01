#!/usr/bin/env bash
# cog.toml's `scopes` list restricts which scopes are valid, but cocogitto
# has no option to make a scope mandatory -- it silently allows commits with
# no scope at all. This guard closes that gap in lefthook's commit-msg hook
# (see lefthook.yml), running alongside `cog verify`.
set -euo pipefail

commit_msg_file="$1"
subject=$(head -n1 "$commit_msg_file")

if [[ "$subject" =~ ^(Merge|fixup!|squash!) ]]; then
  exit 0
fi

if ! [[ "$subject" =~ ^[a-z]+\([a-z0-9_]+\)!?:\ .+ ]]; then
  echo "Commit scope is required, e.g. feat(pathfinder): add denom resolver caching" >&2
  exit 1
fi
