<?php
/**
 * Service contract. Renderers use these six methods; implementations own URLs, colors, and logos.
 */

declare(strict_types=1);

namespace PromariSnsShare\Contracts;

use PromariSnsShare\Domain\Action;
use PromariSnsShare\Domain\ShareRequest;

interface ServiceInterface
{
    /**
 * Lowercase alphanumeric identifier for configuration and tracking.
 */
    public function key(): string;

    /**
 * Default button label, overridable through labels configuration.
 */
    public function label(): string;

    /**
 * Share dialog URL with encoded values from ShareRequest.
 */
    public function shareUrl(ShareRequest $request): string;

    /**
 * Complete SVG logo using currentColor for CSS-controlled color.
 */
    public function icon(): string;

    /**
 * Official brand color in hex; overridable through per-button configuration.
 */
    public function brandColor(): string;

    /**
 * Action performed on click.
 */
    public function action(): Action;
}
