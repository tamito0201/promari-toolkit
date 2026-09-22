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

$assert(str_contains($html, '<promari-sns-share ') && str_contains($html, '</promari-sns-share>'), 'カスタム要素を出す');
$assert(str_contains($html, 'placement="inline"'), '置き場所が属性に入る');
$assert(str_contains($html, 'url="https://example.jp/post/?a=1"'), 'ページの URL が属性に入る');
$assert(str_contains($html, 'title="Hello &amp; World"'), '題名が属性に入り、エスケープされる');
$assert(!str_contains($html, '<?') && !str_contains($html, '<script'), 'HTML に PHP タグやスクリプトが混ざらない');

// 共有 URL の組み立ては Web Component だけが行う。サーバー側には一切現れない。
$assert(!str_contains($html, 'twitter.com') && !str_contains($html, 'facebook.com')
    && !str_contains($html, 'hatena.ne.jp') && !str_contains($html, 'mailto:'),
    'サーバーは共有先のエンドポイントを書き出さない');
$assert(!str_contains($html, 'utm_source') && !str_contains($html, '%3A%2F%2F'),
    'サーバーは UTM も符号化した URL も作らない');
$assert(!str_contains($html, '<a '), 'サーバーはリンクを描かない');

$floating = promari_sns_share(['placement' => 'floating'], false);
$assert(str_contains($floating, 'placement="floating"'), '固定バーの置き場所が属性に入る');
$unknown = promari_sns_share(['placement' => 'nonsense'], false);
$assert(str_contains($unknown, 'placement="inline"'), '未知の置き場所は inline に落とす');

$given = promari_sns_share(['url' => 'https://example.jp/other/', 'title' => 'べつの題'], false);
$assert(str_contains($given, 'url="https://example.jp/other/"') && str_contains($given, 'title="べつの題"'),
    '渡した URL と題名をそのまま使う');

$assert($plugin->config()->scriptUrl() === '', '配布先の URL を生成物から読む（この設定では未指定なので空）');
$assert($plugin->config()->get('placements.floating') === true, '配置の判断に必要な設定を読める');

// Clean up.
(function (string $dir): void {
    foreach (new RecursiveIteratorIterator(new RecursiveDirectoryIterator($dir, FilesystemIterator::SKIP_DOTS), RecursiveIteratorIterator::CHILD_FIRST) as $p) {
        $p->isDir() ? rmdir($p->getPathname()) : unlink($p->getPathname());
    }
    rmdir($dir);
})($tmp);

echo ($fail === 0 ? "✅ render_test: all passed" : "❌ render_test: $fail failed") . PHP_EOL;
exit($fail === 0 ? 0 : 1);
