<?php
/**
 * CLI rendering tests with WordPress stubs and generated configuration. Run with php tools/tests/render_test.php.
 */

declare(strict_types=1);

// Minimal WordPress stubs used by the plugin.
define('ABSPATH', __DIR__ . '/');
function esc_attr(string $s): string { return htmlspecialchars($s, ENT_QUOTES, 'UTF-8'); }
function esc_html(string $s): string { return htmlspecialchars($s, ENT_QUOTES, 'UTF-8'); }
function esc_url(string $s): string { return str_replace(['&', '"', '<', '>'], ['&#038;', '%22', '%3C', '%3E'], $s); } // Encode ampersands as WordPress does.
function esc_url_raw(string $s): string { return $s; }
function esc_attr__(string $s): string { return esc_attr($s); }
function wp_json_encode(mixed $v): string { return json_encode($v, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES); }
function add_action(): void {}
function add_filter(): void {}
function add_shortcode(): void {}
function apply_filters(string $hook, mixed $value): mixed { return $value; }
function get_bloginfo(): string { return 'Promari'; }
function is_singular(): bool { return false; }
function is_admin(): bool { return false; }
function is_feed(): bool { return false; }
function is_ssl(): bool { return true; }
function wp_get_document_title(): string { return 'Hello & World'; }
$_SERVER['HTTP_HOST'] = 'example.jp';
$_SERVER['REQUEST_URI'] = '/post/?a=1';

// Prepare and load generated configuration.
$root = dirname(__DIR__, 2);
$tmp = sys_get_temp_dir() . '/promari-sns-share-test-' . getmypid();
mkdir("$tmp/.config", 0o755, true);
$toml = file_get_contents("$root/config/share_config.example.toml");
$toml = preg_replace('/^plugin_output = ""/m', 'plugin_output = "plugin"', $toml);
$toml = preg_replace('/^web_output = .*$/m', 'web_output = ""', $toml);
$toml = preg_replace('/^web_url = .*$/m', 'web_url = ""', $toml);
$toml = preg_replace('/^enabled = false/m', 'enabled = true', $toml, 1); // utm ON
file_put_contents("$tmp/.config/share_config.toml", $toml);
passthru(escapeshellarg(getenv('PYTHON') ?: 'python3') . ' ' . escapeshellarg("$root/tools/config.py") . ' --config ' . escapeshellarg("$tmp/.config/share_config.toml") . ' --write >/dev/null', $code);
if ($code !== 0) { fwrite(STDERR, "config.py --write failed\n"); exit(1); }
define('PM_SHARE_TEST_CONFIG', "$tmp/plugin/assets/config/share.json");
require "$root/plugin/promari-sns-share.php";

// Assertions.
$fail = 0;
$assert = function (bool $ok, string $label) use (&$fail): void {
    echo ($ok ? '✅ ' : '❌ ') . $label . PHP_EOL;
    $fail += $ok ? 0 : 1;
};
$plugin = promari_sns_share_plugin();
$html = promari_sns_share([], false);

$assert(str_contains($html, 'class="pm-share pm-share--inline pm-share--size-small pm-share--shape-official pm-share--heading-left"'), 'ルート要素に置き場所・外観のクラスが付く');
$assert(substr_count($html, 'pm-share__btn--primary') === 3, '主役ボタンが 3 つ（facebook / x / line）');
$assert(substr_count($html, 'pm-share__btn--secondary') === 5, '補助チャネルが 5 つ（hatena / linkedin / email / copy / native）');
$assert(preg_match('/pm-share__btn--facebook.*?pm-share__btn--x.*?pm-share__btn--line/s', $html) === 1, '並び順が設定どおり');
$assert(str_contains($html, 'data-share="x"'), '計測属性 data-share が付く');
$assert(str_contains($html, 'href="https://twitter.com/intent/tweet?url=https%3A%2F%2Fexample.jp%2Fpost%2F%3Fa%3D1%26utm_source%3Dx%26utm_medium%3Dsocial%26utm_campaign%3Dshare&#038;text=Hello%20%26%20World"'), 'X の URL に UTM 付き URL と本文が RFC 3986 で入る');
$assert(str_contains($html, 'data-pm-share-popup="600x500"'), '小窓の寸法が属性に写る');
$assert(str_contains($html, 'rel="noopener noreferrer nofollow"'), 'rel に nofollow が付く');
$assert(preg_match('/pm-share__btn--native[^>]*hidden/', $html) === 1, 'native は hidden で出す（JS が対応端末でだけ表示）');
$assert(str_contains($html, '<span class="pm-share__label">ポスト</span>'), '文言が labels から入る');
$assert(!str_contains($html, '<?') && !str_contains($html, "\n<script"), 'HTML に PHP タグや不要なスクリプトが混ざらない');

$floating = promari_sns_share(['placement' => 'floating'], false);
$assert(!str_contains($floating, 'pm-share__heading'), '固定バーには見出しを出さない');
$assert(substr_count($floating, 'pm-share__btn--secondary') === 3, '固定バーの補助は floating_secondary_max（3）まで');

$css = (new ReflectionProperty($plugin, 'styles'))->getValue($plugin)->css();
$assert(str_contains($css, '.pm-share__btn--primary{height:20px;') && str_contains($css, 'border-radius:3px}'), 'CSS: small サイズ・公式の角丸');
$assert(str_contains($css, '.pm-share__btn--x{border-radius:9999px'), 'CSS: X だけ丸型');
$js = (new ReflectionProperty($plugin, 'styles'))->getValue($plugin)->js();
$assert(str_contains($js, '"promari-sns-share"') && str_contains($js, 'y>400'), 'JS: イベント名と固定バーの閾値が設定から入る');

// Clean up.
(function (string $dir): void {
    foreach (new RecursiveIteratorIterator(new RecursiveDirectoryIterator($dir, FilesystemIterator::SKIP_DOTS), RecursiveIteratorIterator::CHILD_FIRST) as $p) {
        $p->isDir() ? rmdir($p->getPathname()) : unlink($p->getPathname());
    }
    rmdir($dir);
})($tmp);

echo ($fail === 0 ? "✅ render_test: all passed" : "❌ render_test: $fail failed") . PHP_EOL;
exit($fail === 0 ? 0 : 1);
