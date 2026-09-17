<?php
/**
 * Shared URL construction only. Service-specific parameter choices remain in subclasses.
 */

declare(strict_types=1);

namespace PromariSnsShare\Service;

use PromariSnsShare\Contracts\ServiceInterface;

abstract class AbstractService implements ServiceInterface
{
    /**
 * Build an RFC 3986 query with rawurlencode, omitting empty values. Avoid plus signs for spaces because some services interpret them literally.
 * @param array<string,string> $params
 */
    protected function build(string $endpoint, array $params): string
    {
        $pairs = array_map(
            static fn (string $name, string $value): string => rawurlencode($name) . '=' . rawurlencode($value),
            array_keys($params = array_filter($params, 'strlen')),
            $params
        );
        return $endpoint . ($pairs ? '?' . implode('&', $pairs) : '');
    }
}
