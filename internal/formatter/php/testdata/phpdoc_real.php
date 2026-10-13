<?php

namespace App\Support;

/**
 * @template TKey of array-key
 * @template TValue
 *
 * @phpstan-type Row array{id: int, name: non-empty-string, tags: list<string>}
 */
class TypedRepository
{
    /**
     * @var array<int, Row>
     */
    protected array $rows = [];

    /**
     * @param array{page: int, perPage?: int} $options
     * @param array{sort: string, direction: 'asc'|'desc'} $sorting
     * @return Collection<TKey, TValue>
     *
     * @throws \RuntimeException
     */
    public function query(array $options, array $sorting): Collection
    {
        return new Collection();
    }

    /**
     * @phpstan-assert-if-true non-empty-array<int, Row> $rows
     */
    public function isLoaded(): bool
    {
        return $this->rows !== [];
    }
}
