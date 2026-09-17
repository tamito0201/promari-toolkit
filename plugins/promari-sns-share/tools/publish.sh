#!/usr/bin/env bash
# Publish this component independently; tags and release assets are immutable.
# Run from any directory. --dry-run validates and prints the release plan.
set -euo pipefail
cd "$(dirname "$0")/.."
case "${1:-}" in ""|--dry-run) ;; *) echo 'Usage: tools/publish.sh [--dry-run]' >&2; exit 2;; esac
DRY_RUN=false
[[ "${1:-}" == --dry-run ]] && DRY_RUN=true
PYTHON_BIN="${PYTHON:-python3}"
VERSION=$("$PYTHON_BIN" -c 'import json; print(json.load(open("package.json"))["version"])')
TAG="promari-sns-share-v$VERSION"
REPO="tamito0201/promari-toolkit"
CDN="https://cdn.jsdelivr.net/gh/$REPO@$TAG/plugins/promari-sns-share/dist/promari-sns-share.min.js"
export PYTHON="$PYTHON_BIN"
"$PYTHON_BIN" - "$VERSION" "$CDN" <<'PY'
import sys,tomllib,re
from pathlib import Path
version,url=sys.argv[1:]
config=tomllib.loads(Path('config/share_config.example.toml').read_text())['workflow']
assert re.fullmatch(r'\d+\.\d+\.\d+',version), 'A stable semantic version is required'
assert config['web_version']==version and config['web_url']==url, 'Manifest and configuration versions must match'
assert f' * Version: {version}\n' in Path('plugin/promari-sns-share.php').read_text(), 'PHP plugin version must match'
PY
pnpm run --silent verify
SRI="sha384-$(openssl dgst -sha384 -binary dist/promari-sns-share.min.js | openssl base64 -A)"
if $DRY_RUN; then
  printf 'Would publish %s with WordPress ZIP and JavaScript assets.\n%s\n%s\n' "$TAG" "$CDN" "$SRI"
  exit 0
fi
[[ -z "$(git status --porcelain)" ]] || { echo 'Commit all changes before publishing.' >&2; exit 1; }
if git rev-parse --verify "refs/tags/$TAG" >/dev/null 2>&1; then
  echo "Tag $TAG already exists; publish a new version instead." >&2; exit 1
fi
RELEASE_DIR=$(mktemp -d)
trap 'rm -rf "$RELEASE_DIR"' EXIT
"$PYTHON_BIN" - "$RELEASE_DIR/promari-sns-share-$VERSION.zip" <<'PY'
import sys,zipfile
from pathlib import Path
sys.path.insert(0,str(Path('tools').resolve()))
import config
model=config.load(config.DEFAULT_CONFIG)
with zipfile.ZipFile(sys.argv[1],'w',zipfile.ZIP_DEFLATED) as archive:
    for source in sorted(Path('plugin').rglob('*')):
        if source.is_file(): archive.write(source,Path('promari-sns-share')/source.relative_to('plugin'))
    archive.writestr('promari-sns-share/assets/config/share.json',config.render_json(model))
    archive.write('../../LICENSE','promari-sns-share/LICENSE')
PY
cat > "$RELEASE_DIR/notes.md" <<EOF
Promari SNS Share $VERSION is an independently versioned component of Promari Toolkit.

Install the attached WordPress ZIP, or load the Web Component from the pinned CDN URL:

\`\`\`html
<script type="module" src="$CDN" integrity="$SRI" crossorigin="anonymous"></script>
<promari-sns-share></promari-sns-share>
\`\`\`

The CustomEvent name is \`promari-sns-share\`.
Other components have their own release versions and tags. The source archive contains the full repository; the attached ZIP contains only this WordPress plugin.
EOF
git tag -a "$TAG" -m "Promari SNS Share $VERSION"
git -c credential.helper= -c 'credential.helper=!gh auth git-credential' push --atomic origin HEAD:main "$TAG"
gh release create "$TAG" --repo "$REPO" --verify-tag --latest=false \
  --title "Promari SNS Share $VERSION" --notes-file "$RELEASE_DIR/notes.md" \
  "$RELEASE_DIR/promari-sns-share-$VERSION.zip" dist/promari-sns-share.min.js
printf 'Published %s\n%s\n%s\n' "$TAG" "$CDN" "$SRI"
