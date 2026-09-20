<?php
/**
 * Decide where the share bar appears and emit the custom element. URL construction, styling,
 * and click behaviour belong to the Web Component; this plugin never builds a share URL.
 */

declare(strict_types=1);

namespace PromariSnsShare;

use PromariSnsShare\Config\JsonConfig;
use PromariSnsShare\Contracts\ConfigInterface;
use PromariSnsShare\Domain\Placement;

final class Plugin
{
    /**
 * Track whether the element was emitted so the bundle is loaded only when needed.
 */
    private bool $used = false;

    private function __construct(private readonly ConfigInterface $config)
    {
    }

    public static function boot(string $configPath): self
    {
        return new self(new JsonConfig($configPath));
    }

    public function config(): ConfigInterface
    {
        return $this->config;
    }

    /**
 * Emit the custom element for template tags, shortcodes, automatic insertion, or widgets.
 * Attributes carry the page context only; the component reads the rest from its bundled defaults.
 * @param array{url?:string,title?:string,placement?:string} $args
 */
    public function render(array $args = []): string
    {
        $this->used = true;
        $placement = Placement::parse($args['placement'] ?? null);
        $url = ($args['url'] ?? '') !== '' ? (string) $args['url'] : $this->currentUrl();
        $title = ($args['title'] ?? '') !== '' ? (string) $args['title'] : $this->currentTitle();
        return '<promari-sns-share placement="' . esc_attr($placement->value)
            . '" url="' . esc_attr($url)
            . '" title="' . esc_attr($title) . '"></promari-sns-share>';
    }

    /**
 * Register WordPress hooks.
 */
    public function register(): void
    {
        add_action('wp_footer', function (): void {
            if ($this->config->get('placements.floating') === true && $this->autoTarget()) {
                echo $this->render(['placement' => Placement::Floating->value]);
            }
            $url = $this->config->scriptUrl();
            if ($this->used && $url !== '') {
                echo '<script type="module" id="promari-sns-share-js" src="' . esc_url($url) . '" crossorigin="anonymous"></script>' . "\n";
            }
        }, 30);
        add_shortcode('promari_sns_share', fn ($atts): string => $this->render(shortcode_atts(['url' => '', 'title' => '', 'placement' => 'inline'], (array) $atts, 'promari_sns_share')));
        add_filter('the_content', function (string $content): string {
            if (!$this->autoTarget() || !in_the_loop() || !is_main_query()) {
                return $content;
            }
            $at = fn (Placement $p): string => $this->config->get('placements.' . $p->value) === true ? $this->render(['placement' => $p->value]) : '';
            return $at(Placement::ArticleTop) . $content . $at(Placement::ArticleBottom);
        }, 25);
        if ($this->config->get('placements.sidebar') === true && class_exists('WP_Widget')) {
            require_once __DIR__ . '/Integration/Widget.php';
            Integration\Widget::bind($this->render(...));
            add_action('widgets_init', static fn () => register_widget(Integration\Widget::class));
        }
    }

    /**
 * Check whether automatic placement applies to this singular post type.
 */
    public function autoTarget(): bool
    {
        $types = (array) $this->config->get('placements.post_types');
        return function_exists('is_singular') && !is_admin() && !is_feed() && $types !== [] && is_singular($types);
    }

    private function currentUrl(): string
    {
        $permalink = function_exists('is_singular') && is_singular() ? get_permalink() : '';
        if (is_string($permalink) && $permalink !== '') {
            return $permalink;
        }
        $raw = (function_exists('is_ssl') && is_ssl() ? 'https://' : 'http://') . ($_SERVER['HTTP_HOST'] ?? '') . ($_SERVER['REQUEST_URI'] ?? '/');
        return function_exists('esc_url_raw') ? esc_url_raw($raw) : $raw;
    }

    private function currentTitle(): string
    {
        return function_exists('wp_get_document_title') ? wp_get_document_title() : '';
    }
}
