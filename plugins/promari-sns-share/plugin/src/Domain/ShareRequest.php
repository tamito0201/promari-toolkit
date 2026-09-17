<?php
/**
 * Immutable shared-content value object. TextFormatter has already expanded text and UTM parameters.
 */

declare(strict_types=1);

namespace PromariSnsShare\Domain;

final class ShareRequest
{
    /** @var list<string> */
    public readonly array $hashtags;
    public readonly string $via;

    /** @param list<string> $hashtags */
    public function __construct(
        public readonly string $url,
        public readonly string $title,
        public readonly string $text,
        array $hashtags,
        string $via,
        public readonly string $site,
    ) {
        $this->hashtags = array_values(array_filter(array_map('strval', $hashtags), 'strlen'));
        $this->via = ltrim($via, '@');
    }

    /**
 * Comma-separated hashtags for X; empty when none are configured.
 */
    public function hashtagsCsv(): string
    {
        return implode(',', $this->hashtags);
    }

    /**
 * Copy with a service-specific URL for UTM attribution.
 */
    public function withUrl(string $url): self
    {
        return new self($url, $this->title, $this->text, $this->hashtags, $this->via, $this->site);
    }
}
