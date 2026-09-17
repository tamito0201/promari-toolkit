<?php
/**
 * Pure text and URL formatting. Expand title, site, and URL placeholders and optionally append UTM parameters.
 */

declare(strict_types=1);

namespace PromariSnsShare\Share;

use PromariSnsShare\Contracts\ConfigInterface;
use PromariSnsShare\Domain\ShareRequest;

final class TextFormatter
{
    private const UTM_KEYS = ['source', 'medium', 'campaign', 'content'];

    public function __construct(private readonly ConfigInterface $config)
    {
    }

    /**
 * Create a request from page context before expanding service-specific UTM values.
 */
    public function request(string $url, string $title, string $site): ShareRequest
    {
        return new ShareRequest(
            url: $url,
            title: $title,
            text: self::fill((string) $this->config->get('text.title_template'), $url, $title, $site),
            hashtags: (array) $this->config->get('text.hashtags'),
            via: (string) $this->config->get('text.via'),
            site: $site,
        );
    }

    /**
 * Create the service URL with UTM parameters when enabled.
 */
    public function forService(ShareRequest $request, string $service): ShareRequest
    {
        if ($this->config->get('utm.enabled') !== true) {
            return $request;
        }
        $params = array_filter(
            array_combine(
                array_map(static fn (string $k): string => 'utm_' . $k, self::UTM_KEYS),
                array_map(fn (string $k): string => str_replace('{service}', $service, (string) $this->config->get('utm.' . $k)), self::UTM_KEYS),
            ),
            'strlen'
        );
        return $params ? $request->withUrl(self::appendQuery($request->url, $params)) : $request;
    }

    /**
 * Expand the text template.
 */
    public static function fill(string $template, string $url, string $title, string $site): string
    {
        return strtr($template, ['{title}' => $title, '{site}' => $site, '{url}' => $url]);
    }

    /**
 * Append query parameters while preserving fragments.
 * @param array<string,string> $params
 */
    public static function appendQuery(string $url, array $params): string
    {
        [$base, $fragment] = str_contains($url, '#') ? explode('#', $url, 2) : [$url, null];
        $query = implode('&', array_map(
            static fn (string $k, string $v): string => rawurlencode($k) . '=' . rawurlencode($v),
            array_keys($params),
            $params
        ));
        return $base . (str_contains($base, '?') ? '&' : '?') . $query . ($fragment === null ? '' : '#' . $fragment);
    }
}
