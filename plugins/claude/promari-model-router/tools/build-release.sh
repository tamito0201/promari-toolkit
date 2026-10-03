#!/bin/sh
# Build the release binaries for the given targets only: tools/build-release.sh <targets> [goreleaser flags...]
#
# targets is a comma- or space-separated list of <os>_<arch> (e.g. "linux_amd64" or
# "darwin_arm64,linux_amd64"). The release workflow passes the "targets" input of a manual run, or
# else the repository variable PMR_RELEASE_TARGETS; with neither, nothing is built (there is no
# default list, so a release never builds a platform nobody chose). A target outside the build
# matrix in .goreleaser.yaml is refused by GoReleaser itself ("no binary found"), so the matrix stays
# the only list of what can be built. Binaries land in dist/pmr_<os>_<arch>[.exe], the names bin/pmr
# downloads. Flags after the targets go to every `goreleaser build` (e.g. --snapshot for a local try
# without a release tag).
#
# GoReleaser reads GOOS/GOARCH for --single-target, so it must already be a binary for this host:
# with `go tool` the same variables would cross-compile GoReleaser itself.
set -eu

cd "$(dirname "$0")/.."

raw=${1:-}
[ $# -gt 0 ] && shift
targets=$(printf '%s\n' "$raw" | tr -s ', \t' '\n' | sed '/^$/d')
[ -n "$targets" ] || { echo "❌ no targets: set the repository variable PMR_RELEASE_TARGETS or the \"targets\" input (e.g. linux_amd64)" >&2; exit 1; }
for t in $targets; do
  case "$t" in
    *[!a-z0-9_]* | _* | *_ | *_*_*) echo "❌ invalid target: $t (expected <os>_<arch>, e.g. darwin_arm64)" >&2; exit 1 ;;
    *_*) ;;
    *) echo "❌ invalid target: $t (expected <os>_<arch>, e.g. darwin_arm64)" >&2; exit 1 ;;
  esac
done
dups=$(printf '%s\n' "$targets" | sort | uniq -d)
[ -z "$dups" ] || { echo "❌ duplicate target: $dups" >&2; exit 1; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
plugin=$(basename "$(cd "$(dirname "$0")/../.." && pwd)") # plugins/<plugin>/plugin/tools -> <plugin>
goreleaser=${PMR_TOOL_BIN:-${CLAUDE_PLUGIN_TOOL_BIN:-$HOME/.cache/claude-plugin-tools/$plugin}}/goreleaser
if [ ! -x "$goreleaser" ]; then
  env -u GOOS -u GOARCH go build -modfile=tools/go.mod -o "$work/goreleaser" github.com/goreleaser/goreleaser/v2
  goreleaser=$work/goreleaser
fi

mkdir "$work/out"
for t in $targets; do
  os=${t%_*}
  arch=${t#*_}
  ext=
  [ "$os" = windows ] && ext=.exe
  echo "== building $t"
  GOOS=$os GOARCH=$arch "$goreleaser" build --single-target --clean --output "$work/out/pmr_$t$ext" "$@"
done

rm -rf dist
mkdir dist
mv "$work"/out/pmr_* dist/
echo "✅ built: $(printf '%s\n' "$targets" | tr '\n' ' ')"
