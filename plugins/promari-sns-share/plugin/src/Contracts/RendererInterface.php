<?php
/**
 * Render a button group as HTML without exposing markup details to callers.
 */

declare(strict_types=1);

namespace PromariSnsShare\Contracts;

use PromariSnsShare\Domain\Placement;
use PromariSnsShare\Domain\ShareRequest;

interface RendererInterface
{
    /**
 * Primary buttons use official-style branding; secondary buttons use compact icons.
 * @param list<ServiceInterface> $primary
 * @param list<ServiceInterface> $secondary
 */
    public function render(array $primary, array $secondary, ShareRequest $request, Placement $placement): string;
}
