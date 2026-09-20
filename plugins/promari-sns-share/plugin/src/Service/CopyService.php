<?php
/**
 * Copy the URL with JavaScript; without JavaScript, retain a link to the article.
 */

declare(strict_types=1);

namespace PromariSnsShare\Service;

use PromariSnsShare\Contracts\ServiceInterface;
use PromariSnsShare\Domain\Action;

final class CopyService implements ServiceInterface
{
    public function key(): string
    {
        return 'copy';
    }

    public function label(): string
    {
        return 'URLをコピー';
    }

    public function endpoint(): string
    {
        return '';
    }

    /**
 * Request fields to send, keyed by query parameter name. Empty for services without a dialog.
 * @return array<string,string>
 */
    public function params(): array
    {
        return [];
    }

    public function icon(): string
    {
        return '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="M10.6 13.4a1 1 0 0 1 0-1.4l3.5-3.5a3 3 0 0 1 4.2 4.2l-1.8 1.8a1 1 0 1 1-1.4-1.4l1.8-1.8a1 1 0 0 0-1.4-1.4L12 13.4a1 1 0 0 1-1.4 0zm2.8-2.8a1 1 0 0 1 0 1.4l-3.5 3.5a3 3 0 0 1-4.2-4.2l1.8-1.8a1 1 0 1 1 1.4 1.4l-1.8 1.8a1 1 0 0 0 1.4 1.4l3.5-3.5a1 1 0 0 1 1.4 0z"/></svg>';
    }

    public function brandColor(): string
    {
        return '#5F6368';
    }

    public function action(): Action
    {
        return Action::Copy;
    }
}
