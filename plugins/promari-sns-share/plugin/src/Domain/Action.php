<?php
/**
 * Click actions shared with HTML data attributes and JavaScript.
 */

declare(strict_types=1);

namespace PromariSnsShare\Domain;

enum Action: string
{
    /**
 * Open a share URL in a popup or tab according to configuration.
 */
    case Open = 'open';
    /**
 * Copy the URL through JavaScript, with a normal link fallback.
 */
    case Copy = 'copy';
    /**
 * Open the native share sheet only on supported devices.
 */
    case Native = 'native';
}
