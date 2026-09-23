<?php
/**
 * Hatena Bookmark secondary channel.
 */

declare(strict_types=1);

namespace PromariSnsShare\Destination;

use PromariSnsShare\Contracts\ShareDestinationInterface;
use PromariSnsShare\Domain\ShareAction;

final class HatenaDestination implements ShareDestinationInterface
{
    public function key(): string
    {
        return 'hatena';
    }

    public function label(): string
    {
        return 'はてなブックマーク';
    }

    public function endpoint(): string
    {
        return 'https://b.hatena.ne.jp/entry/panel/';
    }

    /**
 * Request fields to send, keyed by query parameter name. Empty for destinations without a dialog.
 * @return array<string,string>
 */
    public function params(): array
    {
        return ['url' => 'url', 'btitle' => 'title'];
    }

    public function icon(): string
    {
        return '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="M4 4h5.6c2.9 0 4.6 1.3 4.6 3.7 0 1.3-.7 2.3-1.8 2.8 1.6.5 2.5 1.7 2.5 3.4 0 2.7-1.9 4.1-5.2 4.1H4V4zm3.2 5.6h1.9c1.1 0 1.7-.5 1.7-1.4S10.2 6.8 9.1 6.8H7.2v2.8zm0 5.6h2.3c1.3 0 2-.6 2-1.6s-.7-1.6-2-1.6H7.2v3.2zM17.1 4h3.1v9.6h-3.1V4zm1.6 15.9c-1 0-1.8-.8-1.8-1.8s.8-1.8 1.8-1.8 1.8.8 1.8 1.8-.8 1.8-1.8 1.8z"/></svg>';
    }

    public function brandColor(): string
    {
        return '#00A4DE';
    }

    public function action(): ShareAction
    {
        return ShareAction::Open;
    }
}
