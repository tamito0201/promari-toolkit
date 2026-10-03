<?php
/**
 * Sidebar widget, loaded only when WP_Widget is available.
 */

declare(strict_types=1);

namespace PromariSnsShare\Integration;

use Closure;
use WP_Widget;

final class Widget extends WP_Widget
{
    /**
 * Rendering callback bound to Plugin::render.
 * @var Closure(array):string
 */
    private static Closure $renderer;

    /** @param Closure(array):string $renderer */
    public static function bind(Closure $renderer): void
    {
        self::$renderer = $renderer;
    }

    public function __construct()
    {
        parent::__construct('promari_sns_share', 'Promari SNS Share（シェアボタン）', ['description' => '記事の SNS シェアボタンをサイドバーに置く。']);
    }

    public function widget($args, $instance): void
    {
        if (!is_singular() || !isset(self::$renderer)) {
            return;
        }
        $title = trim((string) ($instance['title'] ?? ''));
        echo $args['before_widget']
            . ($title !== '' ? $args['before_title'] . esc_html($title) . $args['after_title'] : '')
            . (self::$renderer)(['placement' => 'sidebar'])
            . $args['after_widget'];
    }

    public function form($instance): void
    {
        $id = esc_attr($this->get_field_id('title'));
        echo "<p><label for=\"$id\">タイトル</label><input class=\"widefat\" id=\"$id\" name=\"" . esc_attr($this->get_field_name('title')) . '" type="text" value="' . esc_attr((string) ($instance['title'] ?? '')) . '"></p>';
    }

    public function update($new_instance, $old_instance): array
    {
        return ['title' => sanitize_text_field((string) ($new_instance['title'] ?? ''))];
    }
}
