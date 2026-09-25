"""Test validation, fail-closed behavior, deployment convergence, and the destination catalog (destinations/*.toml)."""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "tools"))
import config as generator  # noqa: E402

EXAMPLE = ROOT / "config/share_config.example.toml"


def run(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run([sys.executable, str(ROOT / "tools/config.py"), *args], capture_output=True, text=True, check=False)


def patched(text: str, **replacements: str) -> str:
    """Replace key/value lines while retaining comments."""
    for key, value in replacements.items():
        text, count = re.subn(rf"^{re.escape(key)} = .*?( #.*)?$", lambda m: f"{key} = {value}{m.group(1) or ''}", text, count=1, flags=re.M)
        assert count == 1, key
    return text


class CatalogTest(unittest.TestCase):
    def test_extracts_all_bundled_destinations(self) -> None:
        specs = generator.catalog()
        self.assertEqual(set(specs), {"facebook", "x", "line", "hatena", "linkedin", "email", "copy", "native"})
        x = specs["x"]
        self.assertEqual(x.endpoint, "https://twitter.com/intent/tweet")
        self.assertEqual(x.params, {"url": "url", "text": "text", "hashtags": "hashtagsCsv", "via": "via"})
        self.assertEqual(x.action, generator.ShareAction.OPEN)
        self.assertEqual(specs["copy"].action, generator.ShareAction.COPY)
        self.assertEqual(specs["copy"].endpoint, "")
        for spec in specs.values():
            self.assertTrue(spec.icon.startswith("<svg"))
            self.assertRegex(spec.color, r"^#[0-9A-Fa-f]{6}$")

    def test_catalog_order_follows_file_names(self) -> None:
        self.assertEqual(list(generator.catalog()), ["copy", "email", "facebook", "hatena", "line", "linkedin", "native", "x"])


class DestinationFileTest(unittest.TestCase):
    """Each wrong declaration stops generation (fail closed) with a message naming the file."""

    VALID = (
        "key = 'example'\nlabel = 'Example'\nbrand_color = '#123456'\naction = 'open'\n"
        "endpoint = 'https://example.com/share'\nicon = '<svg viewBox=\"0 0 24 24\"><path fill=\"currentColor\" d=\"M0 0h24v24H0z\"/></svg>'\n"
        "\n[params]\nurl = 'url'\ntitle = 'title'\n"
    )

    def setUp(self) -> None:
        self.tmp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmp, True)
        original = generator.DESTINATION_DIR
        self.addCleanup(setattr, generator, "DESTINATION_DIR", original)
        generator.DESTINATION_DIR = self.tmp  # type: ignore[misc]

    def load(self, text: str, name: str = "example.toml") -> dict[str, generator.DestinationSpec]:
        (self.tmp / name).write_text(text, encoding="utf-8")
        return generator.catalog()

    def rejects(self, text: str, message: str, name: str = "example.toml") -> None:
        with self.assertRaisesRegex(generator.ConfigError, message):
            self.load(text, name)

    def test_valid_file_is_read_as_is(self) -> None:
        spec = self.load(self.VALID)["example"]
        self.assertEqual((spec.endpoint, spec.params, spec.action), ("https://example.com/share", {"url": "url", "title": "title"}, generator.ShareAction.OPEN))

    def test_key_must_match_file_name(self) -> None:
        self.rejects(self.VALID, "ファイル名", name="other.toml")

    def test_rejects_invalid_key(self) -> None:
        self.rejects(self.VALID.replace("key = 'example'", "key = 'X-Share'"), "英小文字と数字", name="X-Share.toml")

    def test_rejects_unknown_field(self) -> None:
        self.rejects(self.VALID.replace("action = 'open'", "action = 'open'\ncolour = '#000000'"), "未知")

    def test_rejects_bad_color_and_icon(self) -> None:
        self.rejects(self.VALID.replace("#123456", "red"), "#rrggbb")
        self.rejects(self.VALID.replace("currentColor", "black"), "currentColor")

    def test_rejects_unknown_action(self) -> None:
        self.rejects(self.VALID.replace("action = 'open'", "action = 'print'"), "action は")

    def test_rejects_unknown_request_field(self) -> None:
        self.rejects(self.VALID.replace("title = 'title'", "title = 'author'"), "params は")

    def test_endpoint_and_params_must_agree(self) -> None:
        self.rejects(self.VALID.replace("\n[params]\nurl = 'url'\ntitle = 'title'\n", ""), "params に送る項目")
        self.rejects(self.VALID.replace("https://example.com/share", ""), "params も空")

    def test_rejects_broken_toml(self) -> None:
        self.rejects("key = ", "TOML として読めません")


class ValidationTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmp, True)
        (self.tmp / ".config").mkdir()
        self.source = EXAMPLE.read_text(encoding="utf-8")

    def write(self, text: str) -> Path:
        path = self.tmp / ".config/share_config.toml"
        path.write_text(text, encoding="utf-8")
        return path

    def test_example_is_valid(self) -> None:
        config = generator.load(EXAMPLE)
        self.assertEqual(config.share["destinations"], ["facebook", "x", "line"])
        self.assertEqual(config.enabled_secondary, ["hatena", "linkedin", "email", "copy", "native"])

    def test_rejects_unknown_destination(self) -> None:
        path = self.write(patched(self.source, destinations='["facebook", "instagram"]'))
        with self.assertRaisesRegex(generator.ConfigError, "未知のシェア先"):
            generator.load(path)

    def test_rejects_unknown_key_and_out_of_range(self) -> None:
        with self.assertRaisesRegex(generator.ConfigError, "未知="):
            generator.load(self.write(self.source.replace("[share.style]", "[share.style]\nbogus = 1")))
        with self.assertRaisesRegex(generator.ConfigError, "floating_after_px"):
            generator.load(self.write(patched(self.source, floating_after_px="10")))
        with self.assertRaisesRegex(generator.ConfigError, "accent"):
            generator.load(self.write(patched(self.source, accent='"purple"')))

    def test_rejects_destination_in_both_tiers(self) -> None:
        path = self.write(patched(self.source, destinations='["facebook", "x", "line", "copy"]'))
        with self.assertRaisesRegex(generator.ConfigError, "両方"):
            generator.load(path)

    def test_web_output_requires_url(self) -> None:
        path = self.write(patched(self.source, web_url='""'))
        with self.assertRaisesRegex(generator.ConfigError, "web_output と web_url"):
            generator.load(path)


class DeployTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmp, True)
        (self.tmp / ".config").mkdir()
        text = patched(EXAMPLE.read_text(encoding="utf-8"), plugin_output='"wp-content/plugins/promari-sns-share"', web_output='""', web_url='""')
        self.config = self.tmp / ".config/share_config.toml"
        self.config.write_text(text, encoding="utf-8")

    def test_write_copies_plugin_and_json_then_check_converges(self) -> None:
        result = run("--config", str(self.config), "--write")
        self.assertEqual(result.returncode, 0, result.stderr)
        plugin = self.tmp / "wp-content/plugins/promari-sns-share"
        self.assertTrue((plugin / "promari-sns-share.php").is_file())
        expected = sorted(p.relative_to(ROOT / "plugin") for p in (ROOT / "plugin").rglob("*.php"))
        actual = sorted(p.relative_to(plugin) for p in plugin.rglob("*.php"))
        self.assertEqual(actual, expected)
        document = json.loads((plugin / "assets/config/share.json").read_text(encoding="utf-8"))
        self.assertEqual(document["version"], 1)
        self.assertEqual(document["share"]["destinations"], ["facebook", "x", "line"])
        self.assertEqual(run("--config", str(self.config), "--check").returncode, 0)

    def test_check_detects_stale_artifact(self) -> None:
        run("--config", str(self.config), "--write")
        (self.tmp / "wp-content/plugins/promari-sns-share/assets/config/share.json").write_text("{}", encoding="utf-8")
        result = run("--config", str(self.config), "--check")
        self.assertEqual(result.returncode, 2)
        self.assertIn("古くなっています", result.stderr)

    def test_plugin_output_must_stay_inside_target(self) -> None:
        text = patched(self.config.read_text(encoding="utf-8"), plugin_output='"../outside"')
        self.config.write_text(text, encoding="utf-8")
        result = run("--config", str(self.config), "--write")
        self.assertEqual(result.returncode, 2)
        self.assertIn("target 内", result.stderr)


if __name__ == "__main__":
    unittest.main()
