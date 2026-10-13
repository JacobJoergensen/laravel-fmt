<?php

class Report
{
    /**
     * @return array{
     *     id: int,
     *     label: string,
     *     owner: string|null,
     *     totals: array{open: int, closed: int, pending: int, archived: int, flagged: int, total: int},
     * }
     */
    public function summary(): array
    {
        return [];
    }

    /**
     * @param array{
     *     name: string,
     *     tags: array{
     *         primary: string,
     *         secondary: string,
     *         fallback: string,
     *         extra: string,
     *         more: string,
     *         evenmore: string,
     *     },
     * } $row
     */
    public function ingest(array $row): void {}

    /**
     * @var array{a: int, b: string}
     */
    public array $small = [];

    /**
     * @param array<int, array{
     *     id: int,
     *     name: string,
     *     score: float,
     *     rank: int,
     *     active: bool,
     *     notes: string,
     *     owner: string,
     * }> $rows
     */
    public function bulk(array $rows): void {}
}
