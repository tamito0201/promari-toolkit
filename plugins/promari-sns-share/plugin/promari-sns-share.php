<?php
/**
 * Plugin Name: Promari SNS Share
 * Description: Configurable social share buttons without third-party iframes or SDKs. Supports template tags, shortcodes, automatic placements, widgets, and floating bars.
 * Version: 3.2.0
 * Author: Takaomi Murasaki
 * Requires PHP: 8.1
 */

declare(strict_types=1);

if (!defined('ABSPATH')) {
    exit;
}

/**
 * Load only what running WordPress needs: the placement value object, the configuration contract and reader, and the plugin that emits the custom element. Destinations are declared in destinations/*.toml and compiled into the JavaScript bundle; PHP does not build share URLs.
 */
array_map(
    static fn (string $file): bool => (bool) require_once __DIR__ . '/src/' . $file,
    [
        'Domain/Placement.php',
        'Contracts/ConfigInterface.php',
        'Config/JsonConfig.php',
        'Plugin.php',
    ]
);

/**
 * Plugin entry point used by templates and tests.
 */
function promari_sns_share_plugin(): PromariSnsShare\Plugin
{
    static $plugin = null;
    return $plugin ??= PromariSnsShare\Plugin::boot(__DIR__ . '/assets/config/share.json');
}

if (!function_exists('promari_sns_share')) {
    /**
 * Render share buttons from a theme template. Omitted arguments use the current page.
 * @param array{url?:string,title?:string,placement?:string} $args
 */
    function promari_sns_share(array $args = [], bool $echo = true): string
    {
        $html = promari_sns_share_plugin()->render($args);
        if ($echo) {
            echo $html;
        }
        return $html;
    }
}

promari_sns_share_plugin()->register();
