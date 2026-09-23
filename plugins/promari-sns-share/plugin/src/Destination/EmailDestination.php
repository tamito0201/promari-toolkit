<?php
/**
 * Email secondary channel using mailto.
 */

declare(strict_types=1);

namespace PromariSnsShare\Destination;

use PromariSnsShare\Contracts\ShareDestinationInterface;
use PromariSnsShare\Domain\ShareAction;

final class EmailDestination implements ShareDestinationInterface
{
    public function key(): string
    {
        return 'email';
    }

    public function label(): string
    {
        return 'メールで送る';
    }

    public function endpoint(): string
    {
        return 'mailto:';
    }

    /**
 * Request fields to send, keyed by query parameter name. Empty for destinations without a dialog.
 * @return array<string,string>
 */
    public function params(): array
    {
        return ['subject' => 'text', 'body' => 'url'];
    }

    public function icon(): string
    {
        return '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="M3 5h18a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zm1 2.4V17h16V7.4l-8 5.3-8-5.3zM4.9 7l7.1 4.7L19.1 7H4.9z"/></svg>';
    }

    public function brandColor(): string
    {
        return '#5F6368';
    }

    public function action(): ShareAction
    {
        return ShareAction::Open;
    }
}
