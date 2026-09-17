<?php
/**
 * Read generated share.json and validate its document shape.
 */

declare(strict_types=1);

namespace PromariSnsShare\Config;

use PromariSnsShare\Contracts\ConfigInterface;
use RuntimeException;

final class JsonConfig implements ConfigInterface
{
    /** @var array<string,mixed> */
    private readonly array $data;

    public function __construct(string $path)
    {
        // Only CLI integration tests may override the configuration path.
        $path = PHP_SAPI === 'cli' && defined('PM_SHARE_TEST_CONFIG') ? (string) PM_SHARE_TEST_CONFIG : $path;
        $raw = is_readable($path) ? (string) file_get_contents($path) : '';
        $document = $raw === '' ? null : json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        $share = is_array($document) && ($document['version'] ?? null) === 1 ? ($document['share'] ?? null) : null;
        $this->data = is_array($share)
            ? $share
            : throw new RuntimeException('シェア設定の生成物が不正です。share_config.toml から config.py --write で再生成してください: ' . $path);
    }

    public function get(string $path = ''): mixed
    {
        return self::walk($this->data, $path) ?? throw new RuntimeException('シェア設定が不足しています: ' . $path);
    }

    public function has(string $path): bool
    {
        return self::walk($this->data, $path) !== null;
    }

    /**
 * Traverse a dotted key path; missing entries return null.
 */
    private static function walk(array $node, string $path): mixed
    {
        return array_reduce(
            $path === '' ? [] : explode('.', $path),
            static fn (mixed $carry, string $part): mixed => is_array($carry) && array_key_exists($part, $carry) ? $carry[$part] : null,
            $node
        );
    }
}
