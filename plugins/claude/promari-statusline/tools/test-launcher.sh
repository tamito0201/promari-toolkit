#!/bin/sh
# Test how bin/psl provides a binary, with fake go and curl: tools/test-launcher.sh [path to bin/psl]
#
# Each case copies the launcher into a fresh plugin root with its own checksums.txt and data
# directory, runs `psl version` (or `psl hook`), and checks the exit code, which of download and
# build ran, and what launcher_error says. The fake binary prints PSL_LAUNCHER_SOURCE.
set -u

launcher=${1:-"$(dirname "$0")/../bin/psl"}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/stub" "$work/nogo"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) arch=$(uname -m) ;;
esac
asset=psl_${os}_${arch}

# shellcheck disable=SC2016 # the fake binary expands these when it runs
printf '#!/bin/sh\necho "ran:$PSL_LAUNCHER_SOURCE:$*"\n' >"$work/payload"
if command -v sha256sum >/dev/null 2>&1; then good=$(sha256sum "$work/payload"); else good=$(shasum -a 256 "$work/payload"); fi
good=${good%% *}

cat >"$work/stub/go" <<EOF
#!/bin/sh
echo build >>"$work/calls"
while [ \$# -gt 0 ]; do [ "\$1" = -o ] && { cp "$work/payload" "\$2"; chmod +x "\$2"; }; shift; done
EOF
cat >"$work/stub/curl" <<EOF
#!/bin/sh
echo download >>"$work/calls"
[ "\$CURL_MODE" = fail ] && { echo "curl: (22) The requested URL returned error: 404" >&2; exit 22; }
while [ \$# -gt 0 ]; do [ "\$1" = -o ] && cp "$work/payload" "\$2"; shift; done
EOF
chmod +x "$work/stub/go" "$work/stub/curl"
cp "$work/stub/curl" "$work/nogo/curl"

failed=0

# plugin_root NAME CHECKSUMS(none|other|match|mismatch) GO_MOD(yes|no)
plugin_root() {
  r="$work/$1"
  mkdir -p "$r/bin" "$r/.claude-plugin"
  cp "$launcher" "$r/bin/psl"
  echo '{"version": "0.0.0-test"}' >"$r/.claude-plugin/plugin.json"
  [ "$3" = yes ] && : >"$r/go.mod"
  case $2 in
    other) echo "0000 psl_otheros_otherarch" >"$r/checksums.txt" ;;
    match) echo "$good $asset" >"$r/checksums.txt" ;;
    mismatch) echo "0000 $asset" >"$r/checksums.txt" ;;
  esac
}

# check NAME CHECKSUMS GO(yes|no) CURL_MODE(ok|fail) WANT_RC WANT_CALLS WANT_ERROR(substring, or - for none)
check() {
  plugin_root "$1" "$2" "$3"
  r="$work/$1"
  : >"$work/calls"
  path="$work/stub:/usr/bin:/bin"
  [ "$3" = yes ] || path="$work/nogo:/usr/bin:/bin"
  out=$(CURL_MODE=$4 CLAUDE_PLUGIN_DATA="$r/data" PATH=$path sh "$r/bin/psl" version 2>&1)
  rc=$?
  calls=$(tr '\n' ' ' <"$work/calls" | sed 's/ $//')
  err=$(sed -n '2,$p' "$r/data/launcher_error" 2>/dev/null)
  ok=true
  [ "$rc" = "$5" ] || ok=false
  [ "$calls" = "$6" ] || ok=false
  if [ "$7" = - ]; then [ -z "$err" ] || ok=false; else case $err in *"$7"*) ;; *) ok=false ;; esac; fi
  [ "$5" != 0 ] || [ "$out" = "ran:installed:version" ] || ok=false
  if $ok; then
    echo "✅ $1"
  else
    echo "❌ $1: rc=$rc (want $5), calls=[$calls] (want [$6]), launcher_error=[$err] (want $7), output=[$out]" >&2
    failed=1
  fi
}

check release_binary            match    yes ok   0 "download"       -
check no_release_binary_builds  other    yes ok   0 "build"          -
check download_failure_builds   match    yes fail 0 "download build" -
check checksum_mismatch_stops   mismatch yes ok   1 "download"       "checksum mismatch for $asset"
check no_release_binary_no_go   other    no  ok   1 ""               "checksums.txt has no entry for $asset"
check download_failure_no_go    match    no  fail 1 "download"       "download failed"
check development_checkout      none     yes ok   0 "build"          -

# A hook call returns at once and builds in the background; a later hook call runs the binary.
plugin_root hook other yes
r="$work/hook"
: >"$work/calls"
first=$(PATH="$work/stub:/usr/bin:/bin" CLAUDE_PLUGIN_DATA="$r/data" sh "$r/bin/psl" hook </dev/null 2>&1)
i=0
while [ ! -x "$r/data/bin/psl-0.0.0-test" ] && [ $i -lt 100 ]; do sleep 0.1; i=$((i + 1)); done
second=$(PATH="$work/stub:/usr/bin:/bin" CLAUDE_PLUGIN_DATA="$r/data" sh "$r/bin/psl" hook </dev/null 2>&1)
if [ -z "$first" ] && [ "$second" = "ran:installed:hook" ]; then
  echo "✅ hook_builds_in_background"
else
  echo "❌ hook_builds_in_background: first=[$first] (want empty), second=[$second] (want ran:installed:hook)" >&2
  failed=1
fi

exit $failed
