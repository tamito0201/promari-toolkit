#!/bin/sh
# Run a pinned tool by name: tools/run.sh <tool> [args...]
#
# The modfiles under tools/ are the only place that pins versions. CI installs the official
# release binaries of those exact versions into $PMR_TOOL_BIN (default ~/.cache/pmr-tool-bin, see
# tools/install-ci-tools.sh), and they win when present; everywhere else the tool is built by
# `go tool` from the modfile that declares it, so no caller needs to know which modfile that is.
set -eu

[ $# -ge 1 ] || { echo "usage: tools/run.sh <tool> [args...]" >&2; exit 2; }
name=$1
shift

bin_dir=${PMR_TOOL_BIN:-$HOME/.cache/pmr-tool-bin}
if [ -x "$bin_dir/$name" ]; then
  exec "$bin_dir/$name" "$@"
fi

dir=$(cd "$(dirname "$0")" && pwd)
for modfile in "$dir/go.mod" "$dir"/*/go.mod; do
  # A tool's name is the last element of its package path, skipping a /vN major-version suffix.
  if awk -v want="$name" '
      /^tool \(/ { block = 1; next }
      block && /^\)/ { block = 0; next }
      block || /^tool [^(]/ {
        path = block ? $1 : $2
        n = split(path, part, "/")
        tool = (part[n] ~ /^v[0-9]+$/) ? part[n - 1] : part[n]
        if (tool == want) found = 1
      }
      END { exit !found }' "$modfile"; then
    exec "${GO:-go}" tool -modfile="$modfile" "$name" "$@"
  fi
done

echo "tools/run.sh: no modfile under $dir declares the tool $name" >&2
exit 1
