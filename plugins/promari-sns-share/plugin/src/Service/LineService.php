<?php
/**
 * LINE Share with official widget styling.
 */

declare(strict_types=1);

namespace PromariSnsShare\Service;

use PromariSnsShare\Contracts\ServiceInterface;
use PromariSnsShare\Domain\Action;

final class LineService implements ServiceInterface
{
    public function key(): string
    {
        return 'line';
    }

    public function label(): string
    {
        return 'シェア';
    }

    public function endpoint(): string
    {
        return 'https://social-plugins.line.me/lineit/share';
    }

    /**
 * Request fields to send, keyed by query parameter name. Empty for services without a dialog.
 * @return array<string,string>
 */
    public function params(): array
    {
        return ['url' => 'url', 'text' => 'text'];
    }

    public function icon(): string
    {
        return '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="M12 2.5C6.5 2.5 2 6.1 2 10.6c0 4 3.6 7.4 8.4 8 .3.1.8.2.9.5.1.3.1.7 0 1l-.1.9c0 .3-.2 1 .9.6 1.1-.5 5.9-3.5 8-6 1.5-1.6 2.2-3.2 2.2-5C22.3 6.1 17.5 2.5 12 2.5zM8.3 13.2H6.2c-.3 0-.5-.2-.5-.5V8.6c0-.3.2-.5.5-.5s.5.2.5.5v3.6h1.6c.3 0 .5.2.5.5s-.2.5-.5.5zm1.8-.5c0 .3-.2.5-.5.5s-.5-.2-.5-.5V8.6c0-.3.2-.5.5-.5s.5.2.5.5v4.1zm4.9 0c0 .2-.1.4-.3.5h-.2c-.2 0-.3-.1-.4-.2l-2.1-2.8v2.5c0 .3-.2.5-.5.5s-.5-.2-.5-.5V8.6c0-.2.1-.4.3-.5h.2c.2 0 .3.1.4.2l2.1 2.8V8.6c0-.3.2-.5.5-.5s.5.2.5.5v4.1zm3.3-2.6c.3 0 .5.2.5.5s-.2.5-.5.5h-1.6v1h1.6c.3 0 .5.2.5.5s-.2.5-.5.5h-2.1c-.3 0-.5-.2-.5-.5V8.6c0-.3.2-.5.5-.5h2.1c.3 0 .5.2.5.5s-.2.5-.5.5h-1.6v1h1.6z"/></svg>';
    }

    public function brandColor(): string
    {
        return '#06C755';
    }

    public function action(): Action
    {
        return Action::Open;
    }
}
