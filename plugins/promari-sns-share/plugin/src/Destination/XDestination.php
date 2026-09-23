<?php
/**
 * X Post with the official black pill styling.
 */

declare(strict_types=1);

namespace PromariSnsShare\Destination;

use PromariSnsShare\Contracts\ShareDestinationInterface;
use PromariSnsShare\Domain\ShareAction;

final class XDestination implements ShareDestinationInterface
{
    public function key(): string
    {
        return 'x';
    }

    public function label(): string
    {
        return 'ポスト';
    }

    public function endpoint(): string
    {
        // Use the Twitter intent endpoint for X sharing.
        return 'https://twitter.com/intent/tweet';
    }

    /**
 * Request fields to send, keyed by query parameter name. Empty for destinations without a dialog.
 * @return array<string,string>
 */
    public function params(): array
    {
        return ['url' => 'url', 'text' => 'text', 'hashtags' => 'hashtagsCsv', 'via' => 'via'];
    }

    public function icon(): string
    {
        return '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="M18.2 2.3h3.4l-7.4 8.4 8.7 11.5h-6.8l-5.3-7-6.1 7H1.3l7.9-9L.8 2.3h7l4.8 6.4 5.6-6.4zm-1.2 17.9h1.9L6.7 4.2H4.7l12.3 16z"/></svg>';
    }

    public function brandColor(): string
    {
        return '#000000';
    }

    public function action(): ShareAction
    {
        return ShareAction::Open;
    }
}
