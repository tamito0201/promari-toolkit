<?php
/**
 * Plugin Name: Promari SNS Share
 * Description: Configurable social share buttons without third-party iframes or SDKs. Supports template tags, shortcodes, automatic placements, widgets, and floating bars.
 * Version: 1.1.0
 * Author: Takaomi Murasaki
 * Requires PHP: 8.1
 */

declare(strict_types=1);

if (!defined('ABSPATH')) {
    exit;
}

/**
 * Layered composition: Domain defines values; Contracts define interfaces; Service supplies service metadata; Share formats text and URLs; Render creates markup and styles; Plugin connects WordPress hooks. Generated JSON is validated by the configuration tool.
 */
array_map(
    static fn (string $file): bool => (bool) require_once __DIR__ . '/src/' . $file,
    [
        'Domain/Action.php',
        'Domain/Placement.php',
        'Domain/SharePolicy.php',
        'Domain/ShareRequest.php',
        'Contracts/ConfigInterface.php',
        'Contracts/ServiceInterface.php',
        'Contracts/RendererInterface.php',
        'Config/JsonConfig.php',
        'Service/AbstractService.php',
        'Service/FacebookService.php',
        'Service/XService.php',
        'Service/LineService.php',
        'Service/HatenaService.php',
        'Service/LinkedInService.php',
        'Service/EmailService.php',
        'Service/CopyService.php',
        'Service/NativeService.php',
        'Service/ServiceRegistry.php',
        'Share/TextFormatter.php',
        'Render/ButtonRenderer.php',
        'Render/Styles.php',
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
