<?php
/**
 * Native Web Share API action. Initially hidden; JavaScript reveals it only on supported devices.
 */

declare(strict_types=1);

namespace PromariSnsShare\Destination;

use PromariSnsShare\Contracts\ShareDestinationInterface;
use PromariSnsShare\Domain\ShareAction;

final class NativeDestination implements ShareDestinationInterface
{
    public function key(): string
    {
        return 'native';
    }

    public function label(): string
    {
        return 'その他の共有';
    }

    public function endpoint(): string
    {
        return '';
    }

    /**
 * Request fields to send, keyed by query parameter name. Empty for destinations without a dialog.
 * @return array<string,string>
 */
    public function params(): array
    {
        return [];
    }

    public function icon(): string
    {
        return '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="M12 2.5l4.2 4.2-1.4 1.4-1.8-1.8V15h-2V6.3L9.2 8.1 7.8 6.7 12 2.5zM5 11h4v2H7v7h10v-7h-2v-2h4v11H5V11z"/></svg>';
    }

    public function brandColor(): string
    {
        return '#5F6368';
    }

    public function action(): ShareAction
    {
        return ShareAction::Native;
    }
}
