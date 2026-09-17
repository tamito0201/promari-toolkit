<?php
/**
 * Compose components and wire WordPress hooks. Domain, formatting, and rendering remain independent of WordPress integration.
 */

declare(strict_types=1);

namespace PromariSnsShare;

use PromariSnsShare\Config\JsonConfig;
use PromariSnsShare\Contracts\ConfigInterface;
use PromariSnsShare\Contracts\RendererInterface;
use PromariSnsShare\Contracts\ServiceInterface;
use PromariSnsShare\Domain\Placement;
use PromariSnsShare\Render\ButtonRenderer;
use PromariSnsShare\Render\Styles;
use PromariSnsShare\Service\ServiceRegistry;
use PromariSnsShare\Share\TextFormatter;

final class Plugin
{
    /**
 * Track whether buttons were rendered so scripts are emitted only when needed.
 */
    private bool $used = false;

    private function __construct(
        private readonly ConfigInterface $config,
        private readonly ServiceRegistry $registry,
        private readonly RendererInterface $renderer,
        private readonly Styles $styles,
        private readonly TextFormatter $formatter,
    ) {
    }

    public static function boot(string $configPath): self
    {
        $config = new JsonConfig($configPath);
        // Themes and plugins can register additional ServiceInterface implementations.
        $services = function_exists('apply_filters') ? (array) apply_filters('promari_sns_share_services', ServiceRegistry::builtin()) : ServiceRegistry::builtin();
        $formatter = new TextFormatter($config);
        return new self(
            $config,
            new ServiceRegistry(array_filter($services, static fn (mixed $s): bool => $s instanceof ServiceInterface)),
            new ButtonRenderer($config, $formatter),
            new Styles($config),
            $formatter,
        );
    }

    public function config(): ConfigInterface
    {
        return $this->config;
    }

    public function registry(): ServiceRegistry
    {
        return $this->registry;
    }

    /**
 * Render a button group for template tags, shortcodes, automatic insertion, or widgets.
 * @param array{url?:string,title?:string,placement?:string} $args
 */
    public function render(array $args = []): string
    {
        $this->used = true;
        $placement = Placement::parse($args['placement'] ?? null);
        $request = $this->formatter->request(
            ($args['url'] ?? '') !== '' ? (string) $args['url'] : $this->currentUrl(),
            ($args['title'] ?? '') !== '' ? (string) $args['title'] : $this->currentTitle(),
            function_exists('get_bloginfo') ? (string) get_bloginfo('name') : '',
        );
        $floating = $placement === Placement::Floating;
        $primaryKeys = $floating && (array) $this->config->get('placements.floating_services') !== []
            ? (array) $this->config->get('placements.floating_services')
            : (array) $this->config->get('services');
        $secondaryKeys = $this->enabledSecondary($floating);
        return $this->renderer->render(
            $this->registry->resolve($primaryKeys),
            $this->registry->resolve($floating ? array_slice($secondaryKeys, 0, (int) $this->config->get('placements.floating_secondary_max')) : $secondaryKeys),
            $request,
            $placement,
        );
    }

    /**
 * Register WordPress hooks.
 */
    public function register(): void
    {
        add_action('wp_head', fn () => print('<style id="promari-sns-share-css">' . $this->styles->css() . "</style>\n"), 20);
        add_action('wp_footer', function (): void {
            if ($this->config->get('placements.floating') === true && $this->autoTarget()) {
                echo $this->render(['placement' => Placement::Floating->value]);
            }
            if ($this->used) {
                echo '<script id="promari-sns-share-js">' . $this->styles->js() . "</script>\n";
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

    /**
 * Enabled secondary services in configured order; floating bars honor per-button exclusions.
 * @return list<string> ON
 */
    private function enabledSecondary(bool $floating): array
    {
        return array_keys(array_filter(
            (array) $this->config->get('secondary'),
            fn (mixed $on, string $key): bool => $on === true
                && (!$floating || !$this->config->has("buttons.$key.floating") || $this->config->get("buttons.$key.floating") === true),
            ARRAY_FILTER_USE_BOTH
        ));
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
