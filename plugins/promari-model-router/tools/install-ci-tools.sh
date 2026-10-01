#!/usr/bin/env bash
# Install the official linux/amd64 release binaries of pinned tools into $PMR_TOOL_BIN:
#   tools/install-ci-tools.sh <tool>...   (into $PMR_TOOL_BIN, default $CLAUDE_PLUGIN_TOOL_BIN, else ~/.cache/claude-plugin-tools/<plugin>)
#
# Building these tools from source cost CI most of its time (tools/go.mod alone pulled in over a
# thousand modules). The version of each tool is still read from the modfile that declares it,
# so the modfiles stay the only place that pins versions. Every download is checked against the
# release's own checksum file; tools/run.sh then prefers these binaries.
set -euo pipefail

plugin=$(basename "$(cd "$(dirname "$0")/../.." && pwd)") # plugins/<plugin>/plugin/tools -> <plugin>
PMR_TOOL_BIN=${PMR_TOOL_BIN:-${CLAUDE_PLUGIN_TOOL_BIN:-$HOME/.cache/claude-plugin-tools/$plugin}}
[[ $# -ge 1 ]] || { echo "usage: tools/install-ci-tools.sh <tool>..." >&2; exit 2; }

# tool | GitHub repository | release asset | checksum file | binary inside the archive (empty: the asset is the binary)
# {V} is the version without the leading v, {TAG} with it.
CATALOG='
task|go-task/task|task_linux_amd64.tar.gz|task_checksums.txt|task
golangci-lint|golangci/golangci-lint|golangci-lint-{V}-linux-amd64.tar.gz|golangci-lint-{V}-checksums.txt|golangci-lint
goreleaser|goreleaser/goreleaser|goreleaser_Linux_x86_64.tar.gz|checksums.txt|goreleaser
gitleaks|gitleaks/gitleaks|gitleaks_{V}_linux_x64.tar.gz|gitleaks_{V}_checksums.txt|gitleaks
editorconfig-checker|editorconfig-checker/editorconfig-checker|ec-linux-amd64.tar.gz|checksums.txt|ec-linux-amd64
syft|anchore/syft|syft_{V}_linux_amd64.tar.gz|syft_{V}_checksums.txt|syft
cosign|sigstore/cosign|cosign-linux-amd64|cosign_checksums.txt|
osv-scanner|google/osv-scanner|osv-scanner_linux_amd64|osv-scanner_SHA256SUMS|
octocov|k1LoW/octocov|octocov_{TAG}_linux_amd64.tar.gz|checksums.txt|octocov
actionlint|rhysd/actionlint|actionlint_{V}_linux_amd64.tar.gz|actionlint_{V}_checksums.txt|actionlint
'

dir=$(cd "$(dirname "$0")" && pwd)

# Print the version that the modfile declaring <tool> requires for the module providing it.
pinned_version() {
  local name=$1 modfile
  for modfile in "$dir/go.mod" "$dir"/*/go.mod; do
    awk -v want="$name" '
      /^tool \(/ { block = 1; next }
      block && /^\)/ { block = 0; next }
      block || /^tool [^(]/ {
        path = block ? $1 : $2
        n = split(path, part, "/")
        tool = (part[n] ~ /^v[0-9]+$/) ? part[n - 1] : part[n]
        if (tool == want) pkg = path
        next
      }
      $1 == "require" && $3 ~ /^v[0-9]/ { mod[$2] = $3; next }
      $2 ~ /^v[0-9]/ { mod[$1] = $2 }
      END {
        if (pkg == "") exit 1
        best = ""
        for (m in mod) if ((pkg == m || index(pkg, m "/") == 1) && length(m) > length(best)) best = m
        if (best == "") exit 1
        print mod[best]
      }' "$modfile" && return 0
  done
  return 1
}

mkdir -p "$PMR_TOOL_BIN"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

for name in "$@"; do
  row=$(grep -E "^${name}\|" <<<"$CATALOG") || { echo "❌ $name has no release binary in the catalog" >&2; exit 1; }
  IFS='|' read -r _ repo asset sums inner <<<"$row"
  tag=$(pinned_version "$name") || { echo "❌ no modfile under $dir pins $name" >&2; exit 1; }
  ver=${tag#v}
  asset=${asset//\{V\}/$ver}; asset=${asset//\{TAG\}/$tag}
  sums=${sums//\{V\}/$ver}; sums=${sums//\{TAG\}/$tag}
  base="https://github.com/$repo/releases/download/$tag"

  d="$work/$name"
  mkdir -p "$d"
  curl -fsSL --retry 3 -o "$d/$asset" "$base/$asset"
  curl -fsSL --retry 3 -o "$d/$sums" "$base/$sums"
  want=$(awk -v a="$asset" '{ f = $2; sub(/^\*/, "", f) } f == a { print $1; exit }' "$d/$sums")
  [[ -n "$want" ]] || { echo "❌ $sums has no entry for $asset" >&2; exit 1; }
  got=$(sha256sum "$d/$asset" | awk '{ print $1 }')
  [[ "$got" == "$want" ]] || { echo "❌ checksum mismatch for $asset: got $got, want $want" >&2; exit 1; }

  if [[ -z "$inner" ]]; then
    install -m 0755 "$d/$asset" "$PMR_TOOL_BIN/$name"
  else
    tar -xzf "$d/$asset" -C "$d"
    bin=$(find "$d" -type f -name "$inner" ! -name '*.tar.gz' | head -n 1)
    [[ -n "$bin" ]] || { echo "❌ $asset does not contain $inner" >&2; exit 1; }
    install -m 0755 "$bin" "$PMR_TOOL_BIN/$name"
  fi
  echo "✅ $name $tag (sha256 verified)"
done
