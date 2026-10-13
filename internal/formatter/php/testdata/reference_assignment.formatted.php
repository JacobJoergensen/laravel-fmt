<?php

function replaceAt(array &$tokens, string $path, mixed $replacement): void
{
    $segments = explode('.', $path);
    $key = array_pop($segments);
    $cursor = &$tokens;

    foreach ($segments as $segment) {
        $cursor = &$cursor[$segment];
    }

    $cursor[$key] = $replacement;
}

class Registry
{
    private array $items = [];

    public function bind(string $key): stdClass
    {
        $slot = &$this->items[$key];
        $slot = new stdClass();

        return $slot;
    }
}
