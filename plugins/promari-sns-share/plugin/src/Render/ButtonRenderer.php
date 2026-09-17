<?php
/**
 * Render button markup. Styles own dimensions and colors; services own URL construction.
 */

declare(strict_types=1);

namespace PromariSnsShare\Render;

use PromariSnsShare\Contracts\ConfigInterface;
use PromariSnsShare\Contracts\RendererInterface;
use PromariSnsShare\Contracts\ServiceInterface;
use PromariSnsShare\Domain\Action;
use PromariSnsShare\Domain\Placement;
use PromariSnsShare\Domain\ShareRequest;
use PromariSnsShare\Share\TextFormatter;

final class ButtonRenderer implements RendererInterface
{
    public function __construct(
        private readonly ConfigInterface $config,
        private readonly TextFormatter $formatter,
    ) {
    }

    public function render(array $primary, array $secondary, ShareRequest $request, Placement $placement): string
    {
        $c = $this->config;
        $heading = trim((string) $c->get('heading'));
        $headingPosition = (string) $c->get('appearance.heading_position');
        $classes = implode(' ', [
            'pm-share',
            'pm-share--' . $placement->value,
            'pm-share--size-' . $c->get('appearance.size'),
            'pm-share--shape-' . $c->get('appearance.shape'),
            'pm-share--heading-' . $headingPosition,
        ]);
        $group = static fn (string $class, array $buttons): string => $buttons ? '<span class="' . $class . '">' . implode('', $buttons) . '</span>' : '';
        $button = fn (string $tier) => fn (ServiceInterface $s): string => $this->button($s, $request, $tier);

        return '<div class="' . esc_attr($classes) . '" data-pm-share-placement="' . esc_attr($placement->value) . '" role="group" aria-label="' . esc_attr((string) $c->get('messages.group_label')) . '">'
            . ($heading !== '' && $headingPosition !== 'none' && $placement->showsHeading() ? '<span class="pm-share__heading">' . esc_html($heading) . '</span>' : '')
            . $group('pm-share__primary', array_map($button('primary'), $primary))
            . $group('pm-share__secondary', array_map($button('secondary'), $secondary))
            . '</div>';
    }

    /**
 * Use the per-button override when nonempty, otherwise the default.
 */
    private function option(string $key, string $name, mixed $default): mixed
    {
        $value = $this->config->has("buttons.$key.$name") ? $this->config->get("buttons.$key.$name") : '';
        return $value === '' ? $default : $value;
    }

    private function button(ServiceInterface $service, ShareRequest $request, string $tier): string
    {
        $c = $this->config;
        $key = $service->key();
        $label = $c->has("labels.$key") ? (string) $c->get("labels.$key") : $service->label();
        $tooltip = (string) $this->option($key, 'tooltip', $label);
        $labelStyle = $tier === 'primary' ? (string) $this->option($key, 'label_style', $c->get('appearance.label_style')) : 'icon';
        $href = $service->shareUrl($this->formatter->forService($request, $key));
        $action = $service->action();
        $isOpen = $action === Action::Open;
        $popup = $isOpen && $c->get('behavior.popup') === true && !str_starts_with($href, 'mailto:');

        $attrs = array_filter([
            'class' => "pm-share__btn pm-share__btn--$tier pm-share__btn--$key pm-share__btn--label-$labelStyle",
            'href' => $href,
            (string) $c->get('tracking.attribute') => $key,
            'data-pm-share-action' => $action->value,
            'data-pm-share-url' => $request->url,
            'data-pm-share-title' => $request->title,
            'data-pm-share-popup' => $popup ? (int) $c->get('behavior.popup_width') . 'x' . (int) $c->get('behavior.popup_height') : null,
            'style' => '--pm-share-brand:' . $this->option($key, 'color', $service->brandColor()),
            'title' => $tooltip,
            'aria-label' => $tooltip,
            'target' => $isOpen && $c->get('behavior.open_in_new_tab') === true ? '_blank' : null,
            'rel' => $isOpen ? implode(' ', array_filter(['noopener', 'noreferrer', $c->get('behavior.nofollow') === true ? 'nofollow' : null])) : null,
            'hidden' => $action === Action::Native ? 'hidden' : null,
        ], static fn (mixed $v): bool => $v !== null);

        $attributes = implode('', array_map(
            static fn (string $name, string $value): string => ' ' . $name . '="' . ($name === 'href' ? esc_url($value) : esc_attr($value)) . '"',
            array_keys($attrs),
            array_map('strval', $attrs)
        ));
        return "<a$attributes>"
            . ($labelStyle !== 'text' ? '<span class="pm-share__icon">' . $service->icon() . '</span>' : '')
            . ($labelStyle !== 'icon' ? '<span class="pm-share__label">' . esc_html($label) . '</span>' : '')
            . '</a>';
    }
}
