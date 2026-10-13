<?php

class Registry
{
    public function sweep(): void
    {
        try {
            $this->cache->flush();
        } catch (Throwable) {} finally {}

        foreach ($this->items as $item) {}

        if ($this->locked) {}
    }

    public function noop(): void {}
}

function passthrough(): void {}

$noop = function (): void {};
