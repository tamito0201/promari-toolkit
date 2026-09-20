<?php
/**
 * Emit PHP-side results for parity cases supplied as JSON on stdin. The TypeScript
 * contract test compares its own results against this output.
 */

declare(strict_types=1);

spl_autoload_register(static function (string $class): void {
    $prefix = 'PromariSnsShare\\';
    if (!str_starts_with($class, $prefix)) {
        return;
    }
    $path = dirname(__DIR__, 2) . '/plugin/src/' . str_replace('\\', '/', substr($class, strlen($prefix))) . '.php';
    if (is_file($path)) {
        require $path;
    }
});

use PromariSnsShare\Domain\ShareRequest;
use PromariSnsShare\Domain\SharePolicy;
use PromariSnsShare\Share\TextFormatter;

/**
 * Instantiate every concrete service, keyed as the generated catalog keys them.
 */
function parity_services(): array
{
    $services = [];
    foreach (glob(dirname(__DIR__, 2) . '/plugin/src/Service/*Service.php') ?: [] as $file) {
        $name = basename($file, '.php');
        if (!str_contains((string) file_get_contents($file), 'final class')) {
            continue;
        }
        $class = 'PromariSnsShare\\Service\\' . $name;
        if (!class_exists($class)) {
            continue;
        }
        $service = new $class();
        $services[$service->key()] = $service;
    }
    return $services;
}

$cases = json_decode((string) file_get_contents('php://stdin'), true, 512, JSON_THROW_ON_ERROR);
$services = parity_services();
$result = ['serviceKeys' => array_keys($services), 'shareUrl' => [], 'template' => [], 'appendQuery' => [], 'popup' => [], 'request' => []];

foreach ($cases['shareUrl'] as $case) {
    $request = new ShareRequest(
        url: $case['url'],
        title: $case['title'],
        text: $case['text'],
        hashtags: $case['hashtags'],
        via: $case['via'],
        site: $case['site'],
    );
    $service = $services[$case['service']] ?? null;
    $result['shareUrl'][] = $service === null ? null : $service->shareUrl($request);
}

foreach ($cases['template'] as $case) {
    $result['template'][] = TextFormatter::fill($case['template'], $case['url'], $case['title'], $case['site']);
}

foreach ($cases['appendQuery'] as $case) {
    $result['appendQuery'][] = TextFormatter::appendQuery($case['url'], $case['params']);
}

foreach ($cases['popup'] as $href) {
    $result['popup'][] = SharePolicy::canOpenInPopup($href);
}

foreach ($cases['request'] as $case) {
    $request = new ShareRequest(
        url: $case['url'],
        title: $case['title'],
        text: $case['text'],
        hashtags: $case['hashtags'],
        via: $case['via'],
        site: $case['site'],
    );
    $result['request'][] = ['via' => $request->via, 'hashtags' => $request->hashtags, 'hashtagsCsv' => $request->hashtagsCsv()];
}

echo json_encode($result, JSON_THROW_ON_ERROR | JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
