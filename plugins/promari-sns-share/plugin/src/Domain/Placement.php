<?php
/**
 * Button placements corresponding to configuration keys.
 */

declare(strict_types=1);

namespace PromariSnsShare\Domain;

enum Placement: string
{
    case ArticleTop = 'article_top';
    case ArticleBottom = 'article_bottom';
    case Sidebar = 'sidebar';
    case Floating = 'floating';
    /**
 * Manual placement through template tags or shortcodes.
 */
    case Inline = 'inline';

    /**
 * Parse external placement names, defaulting unknown values to Inline.
 */
    public static function parse(?string $value): self
    {
        return self::tryFrom((string) $value) ?? self::Inline;
    }

    public function showsHeading(): bool
    {
        return $this !== self::Floating;
    }
}
