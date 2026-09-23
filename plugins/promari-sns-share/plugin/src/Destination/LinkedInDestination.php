<?php
/**
 * LinkedIn secondary channel.
 */

declare(strict_types=1);

namespace PromariSnsShare\Destination;

use PromariSnsShare\Contracts\ShareDestinationInterface;
use PromariSnsShare\Domain\ShareAction;

final class LinkedInDestination implements ShareDestinationInterface
{
    public function key(): string
    {
        return 'linkedin';
    }

    public function label(): string
    {
        return 'LinkedIn';
    }

    public function endpoint(): string
    {
        return 'https://www.linkedin.com/sharing/share-offsite/';
    }

    /**
 * Request fields to send, keyed by query parameter name. Empty for destinations without a dialog.
 * @return array<string,string>
 */
    public function params(): array
    {
        return ['u' => 'url'];
    }

    public function icon(): string
    {
        return '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="M6.9 20.5H3.4V9h3.5v11.5zM5.2 7.4C4 7.4 3.1 6.5 3.1 5.4s.9-2 2.1-2 2.1.9 2.1 2-1 2-2.1 2zm15.3 13.1h-3.5v-5.6c0-1.3 0-3-1.9-3s-2.1 1.4-2.1 2.9v5.7H9.5V9h3.4v1.6c.5-.9 1.6-1.9 3.3-1.9 3.6 0 4.2 2.4 4.2 5.4v6.4z"/></svg>';
    }

    public function brandColor(): string
    {
        return '#0A66C2';
    }

    public function action(): ShareAction
    {
        return ShareAction::Open;
    }
}
