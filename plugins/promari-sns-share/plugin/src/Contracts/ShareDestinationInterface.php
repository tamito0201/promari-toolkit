<?php
/**
 * Destination contract. Definitions only. URL construction lives in the Web Component; PHP never builds share URLs.
 */

declare(strict_types=1);

namespace PromariSnsShare\Contracts;

use PromariSnsShare\Domain\ShareAction;

interface ShareDestinationInterface
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
 * Share dialog endpoint, or an empty string for destinations without a dialog.
 */
    public function endpoint(): string;

    /**
 * Request fields to send, keyed by query parameter name.
 * @return array<string,string>
 */
    public function params(): array;

    /**
 * Complete SVG logo using currentColor for CSS-controlled color.
 */
    public function icon(): string;

    /**
 * Official brand color in hex; overridable through per-button configuration.
 */
    public function brandColor(): string;

    /**
 * ShareAction performed on click.
 */
    public function action(): ShareAction;
}
