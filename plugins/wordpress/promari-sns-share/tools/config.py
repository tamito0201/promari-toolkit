#!/usr/bin/env python3
"""Validate share_config.toml and generate reproducible plugin and Web Component outputs.

Use --write to generate outputs, --check to detect drift, and --print to inspect settings.
TOML owns configuration; destinations/*.toml own destination metadata. Invalid settings fail closed."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import tomllib
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass, field
from enum import StrEnum
from pathlib import Path
from typing import Any, Final, Literal

if sys.version_info < (3, 13):  # pragma: no cover
    raise SystemExit("Python 3.13 以上が必要です（tomllib・PEP 695 型・StrEnum を使います）")

ROOT: Final = Path(__file__).resolve().parents[1]
PLUGIN_SOURCE: Final = ROOT / "plugin"
DESTINATION_DIR: Final = ROOT / "destinations"
WEB_SRC: Final = ROOT / "web/src"
WEB_GENERATED: Final = WEB_SRC / "generated"
DEFAULT_CONFIG: Final = ROOT / "config/share_config.example.toml"
PLUGIN_CONFIG_RELATIVE: Final = Path("assets/config/share.json")
WEB_FILENAME: Final = "promari-sns-share.min.js"

HEX_COLOR: Final = re.compile(r"^#[0-9a-fA-F]{6}$")
CSS_COLOR: Final = re.compile(r"^(#[0-9a-fA-F]{3,8}|rgba?\([\d.,\s%]+\)|hsla?\([\d.,\s%]+\)|[a-z]+)$")
SEMVER: Final = re.compile(r"^\d+\.\d+\.\d+$")
DESTINATION_KEY: Final = re.compile(r"^[a-z0-9]+$")
POST_TYPE: Final = re.compile(r"^[a-z0-9_-]+$")
DATA_ATTR: Final = re.compile(r"^data-[a-z][a-z0-9-]*$")
EVENT_NAME: Final = re.compile(r"^[a-z][a-z0-9:_-]*$")

type Json = dict[str, Any]
type Check[T] = Callable[[T], bool]


class ConfigError(ValueError):
    """Configuration error identifying the setting and the reason."""


class Mode(StrEnum):
    WRITE = "write"
    CHECK = "check"
    PRINT = "print"
    GENERATE = "generate"  # Generate TypeScript inputs only, before typechecking or tests.


class ShareAction(StrEnum):
    OPEN = "open"
    COPY = "copy"
    NATIVE = "native"
    COMPOSE = "compose"  # Copy the title and URL, then open the destination's editor (ADR-0003).


class DraftFormat(StrEnum):
    """How a draft template is filled in (ADR-0004): html escapes each value, text inserts it as is."""

    TEXT = "text"
    HTML = "html"


# ------------------------------------------------------------------------------------------
# Small, pure validation helpers
# ------------------------------------------------------------------------------------------
def fields(value: object, required: Mapping[str, type], optional: Mapping[str, type] | None = None, *, label: str) -> Json:
    """Validate table keys and exact types, distinguishing bool from int."""
    optional = optional or {}
    if not isinstance(value, dict):
        raise ConfigError(f"{label}: テーブルが必要です")
    missing = required.keys() - value.keys()
    unknown = value.keys() - required.keys() - optional.keys()
    if missing or unknown:
        raise ConfigError(f"{label}: 不足={sorted(missing)}, 未知={sorted(unknown)}")
    for key, item in value.items():
        expected = (dict(required) | dict(optional))[key]
        if type(item) is not expected:
            raise ConfigError(f"{label}.{key}: {expected.__name__} が必要です（いまは {type(item).__name__}）")
    return value


def ensure[T](value: T, check: Check[T], message: str) -> T:
    if not check(value):
        raise ConfigError(message)
    return value


def one_of(value: str, choices: Sequence[str], label: str) -> str:
    return ensure(value, lambda v: v in choices, f"{label} は {' / '.join(choices)} のいずれかにしてください: {value!r}")


def within(value: int, low: int, high: int, label: str) -> int:
    return ensure(value, lambda v: low <= v <= high, f"{label} は {low}〜{high} にしてください: {value}")


def strings(values: object, label: str, *, pattern: re.Pattern[str] | None = None) -> list[str]:
    if not isinstance(values, list) or any(type(v) is not str for v in values):
        raise ConfigError(f"{label} は文字列の配列にしてください")
    if pattern is not None and any(not pattern.match(v) for v in values):
        raise ConfigError(f"{label} に形式の合わない値があります: {values}")
    return values


# ------------------------------------------------------------------------------------------
# Destination catalog declared as data (destinations/<key>.toml)
# ------------------------------------------------------------------------------------------
REQUEST_FIELDS: Final = frozenset({"url", "title", "text", "via", "site", "hashtagsCsv", "draft"})
PARAM_NAME: Final = re.compile(r"^\w+$")
COMPOSE_ENDPOINT_FORBIDDEN: Final = frozenset(" \t\n\"'<>")
DRAFT_PLACEHOLDERS: Final = ("{title}", "{url}", "{description}", "{image}", "{host}")
DRAFT_PLACEHOLDER: Final = re.compile("|".join(map(re.escape, DRAFT_PLACEHOLDERS)))


@dataclass(frozen=True, slots=True)
class DestinationSpec:
    key: str
    label: str
    color: str
    icon: str
    action: ShareAction
    endpoint: str
    params: dict[str, str] = field(default_factory=dict)
    draft: str | None = None
    draft_format: DraftFormat | None = None
    compose_hint: str | None = None

    def to_json(self) -> Json:
        """Optional fields appear only when declared, so destinations without them generate the same catalog entries."""
        document: Json = {"key": self.key, "label": self.label, "color": self.color, "icon": self.icon, "action": self.action.value, "endpoint": self.endpoint, "params": self.params}
        if self.draft is not None and self.draft_format is not None:
            document["draft"] = {"template": self.draft, "format": self.draft_format.value}
        if self.compose_hint is not None:
            document["composeHint"] = self.compose_hint
        return document


def _draft(name: str, d: Json, action: ShareAction, params: Mapping[str, str]) -> tuple[str | None, DraftFormat | None]:
    """Validate the optional draft template (ADR-0004). Anything that would be silently ignored fails closed."""
    template, format_ = d.get("draft"), d.get("draft_format")
    sends = "draft" in params.values()
    if template is None and format_ is None:
        ensure(sends, lambda s: not s, f"{name}: params で 'draft' を送るなら draft と draft_format を書いてください")
        return None, None
    if template is None or format_ is None:
        raise ConfigError(f"{name}: draft と draft_format は両方書いてください")
    ensure(action, lambda a: a in (ShareAction.OPEN, ShareAction.COMPOSE), f"{name}: draft を使えるのは action = 'open' か 'compose' だけです")
    ensure(template, lambda t: bool(t.strip()), f"{name}: draft を空にしないでください")
    one_of(format_, [f.value for f in DraftFormat], f"{name}: draft_format")
    leftover = DRAFT_PLACEHOLDER.sub("", template)
    ensure(leftover, lambda rest: "{" not in rest and "}" not in rest, f"{name}: draft の差し込み口は {' '.join(DRAFT_PLACEHOLDERS)} だけです（未知の差し込み口か、単独の波括弧があります）")
    if action == ShareAction.OPEN:
        ensure(sends, bool, f"{name}: action = 'open' の draft は、params で 'draft' を送らないと使われません")
    return template, DraftFormat(format_)


def _destination(path: Path) -> DestinationSpec:
    """Read one destination file. It only declares where to send what; no code builds URLs here."""
    name = path.name
    try:
        document = tomllib.loads(path.read_text(encoding="utf-8"))
    except tomllib.TOMLDecodeError as error:
        raise ConfigError(f"{name}: TOML として読めません（{error}）") from error
    d = fields(
        document,
        {"key": str, "label": str, "brand_color": str, "action": str, "endpoint": str, "icon": str},
        {"params": dict, "draft": str, "draft_format": str, "compose_hint": str},
        label=name,
    )
    key = ensure(d["key"], DESTINATION_KEY.match, f"{name}: key は英小文字と数字だけにしてください")
    ensure(key, lambda k: k == path.stem, f"{name}: key（{key}）とファイル名（{path.stem}.toml）をそろえてください")
    ensure(d["label"], bool, f"{name}: label を空にしないでください")
    color = ensure(d["brand_color"], HEX_COLOR.match, f"{name}: brand_color は #rrggbb で書いてください")
    icon = ensure(d["icon"], lambda s: s.startswith("<svg") and "currentColor" in s, f"{name}: icon は <svg …> で始まり fill=\"currentColor\" を使ってください")
    action = ensure(d["action"], lambda a: a in ShareAction.__members__.values(), f"{name}: action は {' / '.join(ShareAction)} のどれかにしてください")
    params = d.get("params", {})
    if any(not PARAM_NAME.match(k) or v not in REQUEST_FIELDS for k, v in params.items()):
        raise ConfigError(f"{name}: params は「SNS 側の項目名 = {' / '.join(sorted(REQUEST_FIELDS))}」の形にしてください")
    if action == ShareAction.COMPOSE:
        # compose opens a fixed editor page; nothing is sent in the URL, so [params] is rejected even when empty.
        ensure(d["endpoint"], lambda e: e.startswith("https://") and COMPOSE_ENDPOINT_FORBIDDEN.isdisjoint(e), f"{name}: action = 'compose' には https:// で始まる投稿画面の URL を endpoint に書いてください（空白・引用符は不可）")
        ensure(d, lambda doc: "params" not in doc, f"{name}: action = 'compose' は URL に項目を送らないので [params] を書かないでください")
    elif d["endpoint"] and not params:
        raise ConfigError(f"{name}: endpoint があるなら params に送る項目を書いてください")
    if not d["endpoint"] and params:
        raise ConfigError(f"{name}: endpoint が空なら params も空にしてください")
    draft, draft_format = _draft(name, d, ShareAction(action), params)
    hint = d.get("compose_hint")
    if hint is not None:
        ensure(action, lambda a: a == ShareAction.COMPOSE, f"{name}: compose_hint は action = 'compose' の共有先にだけ書けます")
        ensure(hint, lambda h: bool(h.strip()) and "\n" not in h, f"{name}: compose_hint は改行を含まない、空でない文字列にしてください")
    return DestinationSpec(
        key=key, label=d["label"], color=color, icon=icon, action=ShareAction(action), endpoint=d["endpoint"], params=dict(params),
        draft=draft, draft_format=draft_format, compose_hint=hint,
    )


def catalog() -> dict[str, DestinationSpec]:
    """Build destination specifications from destinations/*.toml, in file-name order."""
    specs = {spec.key: spec for spec in map(_destination, sorted(DESTINATION_DIR.glob("*.toml")))}
    if not specs:
        raise ConfigError(f"シェア先のカタログが空です: {DESTINATION_DIR}")
    return specs


# ------------------------------------------------------------------------------------------
# Configuration loading and validation
# ------------------------------------------------------------------------------------------
@dataclass(frozen=True, slots=True)
class Workflow:
    target: Path
    plugin_output: str
    web_output: str
    web_url: str
    web_version: str


@dataclass(frozen=True, slots=True)
class Config:
    source: str
    workflow: Workflow
    share: Json
    catalog: dict[str, DestinationSpec]

    @property
    def enabled_secondary(self) -> list[str]:
        return [k for k, on in self.share["secondary"].items() if on]


# Messages added after 4.0.0 are optional so existing configuration files stay valid.
OPTIONAL_MESSAGES: Final[dict[str, str]] = {
    "composed": "記事のリンクをコピーしました。投稿画面に貼り付けてください",
    "compose_failed": "コピーできませんでした。投稿画面にタイトルとURLを入力してください",
}


def _validate_share(share: object, known: Mapping[str, DestinationSpec]) -> Json:
    s = fields(
        share,
        {"destinations": list, "heading": str, "labels": dict, "appearance": dict, "text": dict, "utm": dict, "behavior": dict,
         "placements": dict, "secondary": dict, "tracking": dict, "messages": dict, "style": dict},
        {"buttons": dict},
        label="share",
    )
    names = ", ".join(known)
    destinations = strings(s["destinations"], "share.destinations")
    ensure(destinations, bool, "share.destinations は 1 件以上にしてください")
    ensure(destinations, lambda v: len(set(v)) == len(v), "share.destinations に同じシェア先が 2 回あります")
    for name in destinations:
        ensure(name, known.__contains__, f"share.destinations に未知のシェア先があります: {name}（使える名前: {names}）")
    for key, value in s["labels"].items():
        ensure(key, known.__contains__, f"share.labels に未知のシェア先があります: {key}")
        ensure(value, lambda v: type(v) is str and "\n" not in v, f"share.labels.{key}: 改行を含まない文字列にしてください")
    for key, table in s.get("buttons", {}).items():
        ensure(key, known.__contains__, f"share.buttons に未知のシェア先があります: {key}")
        b = fields(table, {}, {"color": str, "label_style": str, "tooltip": str, "floating": bool}, label=f"share.buttons.{key}")
        if b.get("color", ""):
            ensure(b["color"], HEX_COLOR.match, f"share.buttons.{key}.color は #rrggbb で書いてください")
        if b.get("label_style", ""):
            one_of(b["label_style"], ("icon_text", "icon", "text"), f"share.buttons.{key}.label_style")
    a = fields(s["appearance"], {"size": str, "label_style": str, "shape": str, "gap_px": int, "font_family": str, "heading_position": str, "secondary_size_px": int, "secondary_style": str}, label="share.appearance")
    one_of(a["size"], ("small", "large"), "share.appearance.size")
    one_of(a["label_style"], ("icon_text", "icon", "text"), "share.appearance.label_style")
    one_of(a["shape"], ("official", "pill", "rounded", "square"), "share.appearance.shape")
    within(a["gap_px"], 0, 40, "share.appearance.gap_px")
    one_of(a["heading_position"], ("left", "top", "none"), "share.appearance.heading_position")
    within(a["secondary_size_px"], 20, 48, "share.appearance.secondary_size_px")
    one_of(a["secondary_style"], ("mono", "brand", "outline"), "share.appearance.secondary_style")
    ensure(a["font_family"], lambda v: bool(v.strip()) and ";" not in v and "}" not in v, "share.appearance.font_family は CSS の font-family の値だけを書いてください")
    t = fields(s["text"], {"title_template": str, "hashtags": list, "via": str}, label="share.text")
    strings(t["hashtags"], "share.text.hashtags", pattern=re.compile(r"^[^\s#,]+$"))
    ensure(t["via"], lambda v: not v.startswith("@") and " " not in v, "share.text.via は @ を付けずに書いてください")
    fields(s["utm"], {"enabled": bool, "source": str, "medium": str, "campaign": str, "content": str}, label="share.utm")
    b = fields(s["behavior"], {"open_in_new_tab": bool, "popup": bool, "popup_width": int, "popup_height": int, "nofollow": bool}, label="share.behavior")
    within(b["popup_width"], 300, 1200, "share.behavior.popup_width")
    within(b["popup_height"], 300, 1000, "share.behavior.popup_height")
    p = fields(
        s["placements"],
        {"post_types": list, "article_top": bool, "article_bottom": bool, "sidebar": bool, "floating": bool, "floating_position": str,
         "floating_after_px": int, "floating_secondary_max": int, "floating_hide_near_end": bool, "floating_destinations": list},
        label="share.placements",
    )
    strings(p["post_types"], "share.placements.post_types", pattern=POST_TYPE)
    one_of(p["floating_position"], ("bottom", "top"), "share.placements.floating_position")
    within(p["floating_after_px"], 100, 5000, "share.placements.floating_after_px")
    within(p["floating_secondary_max"], 0, 8, "share.placements.floating_secondary_max")
    for name in strings(p["floating_destinations"], "share.placements.floating_destinations"):
        ensure(name, known.__contains__, f"share.placements.floating_destinations に未知のシェア先があります: {name}")
    for key, value in s["secondary"].items():
        ensure(key, known.__contains__, f"share.secondary に未知のシェア先があります: {key}")
        ensure(value, lambda v: type(v) is bool, f"share.secondary.{key} は true / false で書いてください")
        ensure(key, lambda k: k not in destinations, f"{key} が destinations と secondary の両方にあります（どちらか一方にしてください）")
    tr = fields(s["tracking"], {"attribute": str, "event_name": str}, label="share.tracking")
    ensure(tr["attribute"], DATA_ATTR.match, "share.tracking.attribute は data-… の形にしてください")
    ensure(tr["event_name"], EVENT_NAME.match, "share.tracking.event_name は英小文字で始めてください")
    fields(s["messages"], {"copied": str, "group_label": str}, {name: str for name in OPTIONAL_MESSAGES}, label="share.messages")
    st = fields(s["style"], {"accent": str, "floating_background": str}, label="share.style")
    ensure(st["accent"], HEX_COLOR.match, "share.style.accent は #rrggbb で書いてください")
    ensure(st["floating_background"], CSS_COLOR.match, "share.style.floating_background は CSS の色にしてください")
    return s


def load(path: Path) -> Config:
    path = path.expanduser().resolve()
    document = fields(tomllib.loads(path.read_text(encoding="utf-8")), {"version": int, "workflow": dict, "share": dict}, label="設定")
    ensure(document["version"], lambda v: v == 1, "version は 1 だけを受け付けます")
    w = fields(document["workflow"], {"target": str, "plugin_output": str, "web_output": str, "web_url": str}, {"web_version": str}, label="workflow")
    version = ensure(w.get("web_version", "1.0.0"), SEMVER.match, "workflow.web_version は 1.2.3 の形で書いてください")
    ensure((w["web_output"], w["web_url"]), lambda pair: bool(pair[0]) == bool(pair[1]), "workflow.web_output と web_url は両方書くか両方空にしてください")
    ensure(w["web_url"], lambda v: v == "" or v.startswith("https://"), "workflow.web_url は https:// で始めてください")
    root = path.parent.parent if path.parent.name in {".config", "config"} else path.parent
    known = catalog()
    return Config(
        source=str(path.relative_to(root)) if path.is_relative_to(root) else path.name,
        workflow=Workflow(
            target=(root / Path(w["target"]).expanduser()).resolve(),
            plugin_output=w["plugin_output"],
            web_output=w["web_output"],
            web_url=w["web_url"],
            web_version=version,
        ),
        share=_validate_share(document["share"], known),
        catalog=known,
    )


# ------------------------------------------------------------------------------------------
# Generated outputs
# ------------------------------------------------------------------------------------------
def render_json(config: Config) -> bytes:
    payload = json.dumps(config.share, ensure_ascii=False, sort_keys=True, indent=2, allow_nan=False)
    document = {"version": 1, "source": config.source, "sha256": hashlib.sha256(payload.encode()).hexdigest(),
                "cdn": {"url": config.workflow.web_url}, "share": config.share}
    return (json.dumps(document, ensure_ascii=False, indent=2, allow_nan=False) + "\n").encode("utf-8")


def web_defaults(config: Config) -> Json:
    """Convert validated settings to Web Component defaults."""
    s = config.share
    p = s["placements"]
    return {
        "destinations": s["destinations"],
        "secondary": config.enabled_secondary,
        "labels": s["labels"],
        "heading": s["heading"],
        "buttons": s.get("buttons", {}),
        "appearance": s["appearance"],
        "text": s["text"],
        "utm": s["utm"],
        "behavior": s["behavior"],
        "floating": {
            "position": p["floating_position"],
            "after": p["floating_after_px"],
            "secondaryMax": p["floating_secondary_max"],
            "hideNearEnd": p["floating_hide_near_end"],
            "destinations": p["floating_destinations"],
        },
        "tracking": s["tracking"],
        "messages": s["messages"] | {k: v for k, v in OPTIONAL_MESSAGES.items() if k not in s["messages"]},
        "style": s["style"],
    }


def _compact(value: object) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"))


def generated_sources(config: Config) -> dict[Path, str]:
    """Generate TypeScript modules from the PHP catalog and TOML defaults."""
    header = "// Generated by tools/config.py; do not edit.\n"
    return {
        WEB_GENERATED / "catalog.ts": header
        + "import type { ShareDestinationDefinition } from '../application/ShareButtonCatalog.ts';\n"
        + "export const CATALOG: readonly ShareDestinationDefinition[] = Object.freeze(" + _compact([spec.to_json() for spec in config.catalog.values()]) + " as const satisfies readonly ShareDestinationDefinition[]);\n",
        WEB_GENERATED / "defaults.ts": header
        + "import type { ShareSettings } from '../application/ShareSettings.ts';\n"
        + "export const DEFAULTS: ShareSettings = Object.freeze(" + _compact(web_defaults(config)) + " satisfies ShareSettings);\n"
        + f"export const VERSION = {_compact(config.workflow.web_version)} as const;\nexport const WEB_URL = {_compact(config.workflow.web_url)} as const;\n",
    }


def build_web(config: Config) -> bytes:
    """Bundle the four layers with esbuild; deterministic output enables drift checks."""
    for path, text in generated_sources(config).items():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
    esbuild = ROOT / "node_modules/.bin/esbuild"
    if not esbuild.is_file():
        raise ConfigError("esbuild is missing; run pnpm install --frozen-lockfile in this directory")
    banner = f"/*! Promari SNS Share v{config.workflow.web_version} — <promari-sns-share> Web Components | https://github.com/tamito0201/promari-toolkit | MIT */"
    result = subprocess.run(
        [str(esbuild), str(WEB_SRC / "index.ts"), "--bundle", "--minify", "--format=iife", "--target=es2022",
         "--legal-comments=none", f"--banner:js={banner}", "--log-level=warning"],
        capture_output=True, text=True, check=False,
    )
    if result.returncode != 0:
        raise ConfigError("esbuild が失敗しました: " + result.stderr.strip())
    return (result.stdout.rstrip("\n") + "\n").encode("utf-8")


def _die(message: str) -> str:
    raise ConfigError(message)


def _inside(root: Path, relative: str, what: str) -> Path:
    result = (root / Path(relative)).resolve()
    ensure(result, lambda r: not Path(relative).is_absolute() and r.is_relative_to(root) and r != root, f"{what} は target 内の相対ディレクトリにしてください")
    return result


def plugin_files() -> dict[Path, bytes]:
    files = {p.relative_to(PLUGIN_SOURCE): p.read_bytes() for p in sorted(PLUGIN_SOURCE.rglob("*.php")) if "__pycache__" not in p.parts}
    return files or _die(f"プラグインの正本が見つかりません: {PLUGIN_SOURCE}")  # type: ignore[return-value]


def deploy_targets(config: Config) -> dict[Path, bytes]:
    """Map absolute output paths to plugin, JSON configuration, and JavaScript contents."""
    w = config.workflow
    targets: dict[Path, bytes] = {}
    if w.plugin_output:
        plugin_dir = _inside(w.target, w.plugin_output, "workflow.plugin_output")
        targets |= {plugin_dir / rel: data for rel, data in plugin_files().items()}
        targets[plugin_dir / PLUGIN_CONFIG_RELATIVE] = render_json(config)
    if w.web_output:
        targets[_inside(w.target, w.web_output, "workflow.web_output") / WEB_FILENAME] = build_web(config)
    return targets or _die("workflow.plugin_output か workflow.web_output を指定してください")  # type: ignore[return-value]


def describe(config: Config) -> str:
    s, w = config.share, config.workflow
    label = lambda k: s["labels"].get(k, config.catalog[k].label)  # noqa: E731
    on = [k for k in ("article_top", "article_bottom", "sidebar", "floating") if s["placements"][k]]
    return "\n".join([
        "主役ボタン: " + " / ".join(f"{k}（{label(k)}）" for k in s["destinations"]),
        "補助チャネル: " + (", ".join(config.enabled_secondary) or "なし"),
        f"外観: size={s['appearance']['size']} label={s['appearance']['label_style']} shape={s['appearance']['shape']} secondary={s['appearance']['secondary_style']}",
        f"動線: {', '.join(on) or '自動挿入なし'}（対象 post_types={s['placements']['post_types']}）",
        f"共有文: {s['text']['title_template']!r} hashtags={s['text']['hashtags']} via={s['text']['via'] or '-'} utm={'ON' if s['utm']['enabled'] else 'OFF'}",
        f"計測: 属性 {s['tracking']['attribute']} / イベント {s['tracking']['event_name']}",
        "配布先: " + (w.plugin_output or "（プラグイン配布なし）") + " / Web Components: " + (f"{w.web_output} → {w.web_url} (v{w.web_version})" if w.web_output else "（作らない）"),
    ])


def write_atomically(destination: Path, payload: bytes) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode="wb", dir=destination.parent, delete=False) as stream:
        stream.write(payload)
        temporary = Path(stream.name)
    try:
        temporary.chmod(0o644)
        os.replace(temporary, destination)
    finally:
        temporary.unlink(missing_ok=True)


def run(mode: Mode, config_path: Path) -> int:
    config = load(config_path)
    match mode:
        case Mode.PRINT:
            print(describe(config))
        case Mode.CHECK:
            stale = [str(dst) for dst, payload in deploy_targets(config).items() if not dst.exists() or dst.read_bytes() != payload]
            if stale:
                raise ConfigError("配布物が古くなっています。--write で再生成してください: " + ", ".join(stale))
            print(f"✅ 配布物は最新です: {config_path}\n{describe(config)}")
        case Mode.WRITE:
            for destination, payload in deploy_targets(config).items():
                write_atomically(destination, payload)
            print(f"✅ 書き出しました: {config_path}\n{describe(config)}")
        case Mode.GENERATE:
            # Generate TypeScript inputs without building distribution files or requiring esbuild.
            for destination, text in generated_sources(config).items():
                write_atomically(destination, text.encode("utf-8"))
            print(f"✅ 型スタブを生成しました（web/src/generated）: {config_path}")
    return 0


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--config", type=Path, default=DEFAULT_CONFIG)
    group = parser.add_mutually_exclusive_group(required=True)
    for mode in Mode:
        group.add_argument(f"--{mode}", dest="mode", action="store_const", const=mode)
    args = parser.parse_args(argv)
    try:
        return run(args.mode, args.config)
    except (ConfigError, OSError, tomllib.TOMLDecodeError) as error:
        print(f"❌ シェア設定が不正です: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
