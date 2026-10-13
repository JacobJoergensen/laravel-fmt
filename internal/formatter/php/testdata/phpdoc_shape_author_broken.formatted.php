<?php

/**
 * @phpstan-type ItemMeta array{
 *     item_key: string,
 *     tags: list<string>,
 * }
 * @phpstan-type Flat array{item_key: string, tags: list<string>}
 * @phpstan-type Pair array{
 *     id: int,
 * }|array{
 *     name: string,
 * }
 * @phpstan-type Quoted array{label: 'first
 *     second', id: int}
 */
final class Registry
{
    /**
     * @return array{
     *     total: int,
     * }|null
     */
    public function totals(): ?array
    {
        return null;
    }
}
