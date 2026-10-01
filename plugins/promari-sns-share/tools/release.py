#!/usr/bin/env python3
"""Release files of Promari SNS Share, used by the release workflow.

    python3 tools/release.py check <version>   # every place that carries the version agrees
    python3 tools/release.py zip <version>     # dist/promari-sns-share-<version>.zip (WordPress plugin)
    python3 tools/release.py sri               # Subresource Integrity hash of the JavaScript bundle

The ZIP is reproducible: entries are sorted and carry a fixed timestamp and mode, so the same sources
always give the same bytes and the same line in checksums.txt.
"""

from __future__ import annotations

import base64
import hashlib
import json
import re
import sys
import tomllib
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tools"))

import config  # noqa: E402

NAME = "promari-sns-share"
BUNDLE = ROOT / "dist" / f"{NAME}.min.js"
VERSION_RE = re.compile(r"\d+\.\d+\.\d+")
ZIP_TIME = (1980, 1, 1, 0, 0, 0)


def cdn_url(version: str) -> str:
    return f"https://cdn.jsdelivr.net/gh/tamito0201/promari-toolkit@{NAME}-v{version}/plugins/{NAME}/dist/{NAME}.min.js"


def versions() -> dict[str, str]:
    """Return the version found in each place that carries it."""
    workflow = tomllib.loads((ROOT / "config" / "share_config.example.toml").read_text(encoding="utf-8"))["workflow"]
    header = re.search(r"^ \* Version: (\S+)$", (ROOT / "plugin" / f"{NAME}.php").read_text(encoding="utf-8"), re.M)
    return {
        "package.json": json.loads((ROOT / "package.json").read_text(encoding="utf-8"))["version"],
        "pyproject.toml": tomllib.loads((ROOT / "pyproject.toml").read_text(encoding="utf-8"))["project"]["version"],
        f"plugin/{NAME}.php": header.group(1) if header else "(no Version header)",
        "config web_version": workflow.get("web_version", ""),
        "config web_url": workflow.get("web_url", ""),
    }


def check(version: str) -> list[str]:
    """Return the mismatches between version and the files that carry it (empty when they agree)."""
    if not VERSION_RE.fullmatch(version):
        return [f"{version}: a stable version <major>.<minor>.<patch> is required"]
    want = {key: version for key in versions()}
    want["config web_url"] = cdn_url(version)
    return [f"{key} is {got!r}, expected {want[key]!r}" for key, got in versions().items() if got != want[key]]


def build_zip(version: str) -> Path:
    """Write the WordPress plugin ZIP: plugin/, the generated share.json and the license."""
    out = ROOT / "dist" / f"{NAME}-{version}.zip"
    entries: list[tuple[str, bytes]] = [
        (f"{NAME}/{path.relative_to(ROOT / 'plugin').as_posix()}", path.read_bytes())
        for path in sorted((ROOT / "plugin").rglob("*"))
        if path.is_file() and "__pycache__" not in path.parts and path.name != ".DS_Store"
    ]
    entries.append((f"{NAME}/assets/config/share.json", config.render_json(config.load(config.DEFAULT_CONFIG))))
    entries.append((f"{NAME}/LICENSE", (ROOT / "LICENSE").read_bytes()))
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as archive:
        for name, data in sorted(entries):
            info = zipfile.ZipInfo(name, ZIP_TIME)
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o644 << 16
            archive.writestr(info, data)
    return out


def sri() -> str:
    return "sha384-" + base64.b64encode(hashlib.sha384(BUNDLE.read_bytes()).digest()).decode("ascii")


def main(argv: list[str]) -> int:
    if len(argv) == 2 and argv[0] == "check":
        problems = check(argv[1])
        for problem in problems:
            print(f"❌ {problem}", file=sys.stderr)
        if not problems:
            print(f"✅ every version is {argv[1]}")
        return 1 if problems else 0
    if len(argv) == 2 and argv[0] == "zip":
        problems = check(argv[1])
        if problems:
            print(f"❌ versions disagree; run tools/release.py check {argv[1]}", file=sys.stderr)
            return 1
        print(build_zip(argv[1]).relative_to(ROOT))
        return 0
    if argv == ["sri"]:
        print(sri())
        return 0
    print(__doc__.split("\n\n")[1], file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
