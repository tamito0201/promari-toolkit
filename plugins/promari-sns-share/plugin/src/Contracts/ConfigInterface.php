<?php
/**
 * Configuration read contract, independent of the storage format.
 */

declare(strict_types=1);

namespace PromariSnsShare\Contracts;

interface ConfigInterface
{
    /**
 * Read a dotted configuration path. Missing values throw instead of silently falling back.
 * @throws \RuntimeException
 */
    public function get(string $path = ''): mixed;

    /**
 * Check whether an optional setting exists.
 */
    public function has(string $path): bool;
}
