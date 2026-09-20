<?php
/**
 * Rules that belong to sharing itself, independent of rendering and configuration.
 */

declare(strict_types=1);

namespace PromariSnsShare\Domain;

final class SharePolicy
{
    /**
 * Whether the share target can open in a popup. mailto: hands the request to a mail client, so a popup would be left empty.
 * Keep this in step with canOpenInPopup in the TypeScript domain.
 */
    public static function canOpenInPopup(string $href): bool
    {
        return !str_starts_with($href, 'mailto:');
    }
}
