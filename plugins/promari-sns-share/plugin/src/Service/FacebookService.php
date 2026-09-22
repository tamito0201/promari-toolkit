<?php
/**
 * Facebook Share with official small-widget styling.
 */

declare(strict_types=1);

namespace PromariSnsShare\Service;

use PromariSnsShare\Contracts\ServiceInterface;
use PromariSnsShare\Domain\Action;

final class FacebookService implements ServiceInterface
{
    public function key(): string
    {
        return 'facebook';
    }

    public function label(): string
    {
        return 'シェア';
    }

    public function endpoint(): string
    {
        return 'https://www.facebook.com/sharer/sharer.php';
    }

    /**
 * Request fields to send, keyed by query parameter name. Empty for services without a dialog.
 * @return array<string,string>
 */
    public function params(): array
    {
        return ['u' => 'url'];
    }

    public function icon(): string
    {
        return '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="currentColor" d="M13.6 22v-8.2h2.8l.4-3.2h-3.2V8.5c0-.9.3-1.6 1.6-1.6H17V4.1c-.3 0-1.3-.1-2.5-.1-2.5 0-4.2 1.5-4.2 4.3v2.3H7.5v3.2h2.8V22h3.3z"/></svg>';
    }

    public function brandColor(): string
    {
        return '#1877F2';
    }

    public function action(): Action
    {
        return Action::Open;
    }
}
