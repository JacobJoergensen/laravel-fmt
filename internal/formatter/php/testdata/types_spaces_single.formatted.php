<?php

namespace App;

/**
 * @property-read Alpha | null $first
 */
class Example
{
    public const int | string LIMIT = 1;

    /**
     * @var array<string, Alpha | Beta> | Closure
     */
    private array | Closure $items;

    public function __construct(
        private readonly int | string $id,
    ) {}

    /**
     * @param  Alpha & Beta  $both
     * @param  array{name: string | int, tags: list<Alpha | Beta>}  $shape
     * @return (Alpha & Beta) | null
     */
    public function run(
        Alpha & Beta $both,
        (Alpha & Beta) | null $either,
        array $shape,
        ?int $count,
    ): string | int | null {
        $map = fn (int | string $key): int | float => $key;
        $call = function (Alpha | Beta $value): Alpha & Beta {
            return $value;
        };

        try {
            $flags = $count | 4;
            $mask = $count & 2;
        } catch (FirstException | SecondException $e) {
            return null;
        }

        return $flags;
    }
}
