<?php
/**
 * Immutable service lookup preserving configured order. Unknown names throw; the generator validates the same catalog.
 */

declare(strict_types=1);

namespace PromariSnsShare\Service;

use PromariSnsShare\Contracts\ServiceInterface;
use RuntimeException;

final class ServiceRegistry
{
    /** @var array<string,ServiceInterface> */
    private readonly array $services;

    /** @param iterable<ServiceInterface> $services */
    public function __construct(iterable $services)
    {
        $this->services = array_reduce(
            [...$services],
            static function (array $carry, ServiceInterface $service): array {
                preg_match('/^[a-z0-9]+$/', $service->key())
                    || throw new RuntimeException('シェア先の識別子は英小文字と数字だけにしてください: ' . $service->key());
                return $carry + [$service->key() => $service];
            },
            []
        );
    }

    /**
 * Bundled services, extensible through the promari_sns_share_services filter.
 * @return list<ServiceInterface>
 */
    public static function builtin(): array
    {
        return [
            new FacebookService(),
            new XService(),
            new LineService(),
            new HatenaService(),
            new LinkedInService(),
            new EmailService(),
            new CopyService(),
            new NativeService(),
        ];
    }

    /**
 * Resolve service keys in configuration order.
 * @param list<string> $keys
 * @return list<ServiceInterface>
 */
    public function resolve(array $keys): array
    {
        return array_map(
            fn (string $key): ServiceInterface => $this->services[$key]
                ?? throw new RuntimeException('未知のシェア先です（promari_sns_share_services で登録してください）: ' . $key),
            array_values($keys)
        );
    }

    /** @return list<string> */
    public function keys(): array
    {
        return array_keys($this->services);
    }
}
