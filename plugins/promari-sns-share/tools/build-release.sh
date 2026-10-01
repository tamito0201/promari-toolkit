#!/usr/bin/env bash
# Build the release files of one version into dist/ and write checksums.txt and sbom.spdx.json:
#   tools/build-release.sh <version>
#
# Run after `pnpm install --frozen-lockfile` and `pnpm run verify`, which fails when the committed
# dist/promari-sns-share.min.js no longer matches the sources. The bundle is attached as it is
# committed, because jsDelivr serves the committed file at the release tag.
#
# The SBOM lists the packages bundled into the JavaScript (the production dependencies): they are
# installed alone into a temporary directory and read by syft ($SYFT, default: syft on PATH) as installed packages. The
# development tools (TypeScript, esbuild) and the lockfile are not part of what is distributed.
set -euo pipefail
cd "$(dirname "$0")/.."

version=${1:-}
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "usage: tools/build-release.sh <major>.<minor>.<patch>" >&2; exit 2; }
python=${PYTHON:-python3}
syft=${SYFT:-syft}
command -v "$syft" >/dev/null || { echo "❌ syft not found ($syft); set SYFT or put it on PATH" >&2; exit 1; }

"$python" tools/release.py zip "$version" >/dev/null
assets=("promari-sns-share.min.js" "promari-sns-share-$version.zip")
(cd dist && sha256sum "${assets[@]}" | sed 's|  | |') > checksums.txt

prod=$(mktemp -d)
trap 'rm -rf "$prod"' EXIT
cp package.json pnpm-lock.yaml "$prod/"
pnpm install --dir "$prod" --prod --frozen-lockfile --ignore-scripts --ignore-workspace --silent
SYFT_CHECK_FOR_APP_UPDATE=false "$syft" scan "dir:$prod/node_modules" --quiet \
  --override-default-catalogers javascript-package-cataloger \
  --source-name promari-sns-share --source-version "$version" \
  -o spdx-json=sbom.spdx.json
packages=$("$python" -c 'import json,sys; print(len(json.load(open(sys.argv[1]))["packages"]))' sbom.spdx.json)
[[ $packages -gt 0 ]] || { echo "❌ the SBOM lists no packages" >&2; exit 1; }

cat checksums.txt
echo "sbom.spdx.json: $packages packages"
