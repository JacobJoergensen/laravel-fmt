<?php

class Plan
{
    /**
     * @return array{
     *     header: string,
     *     elements: Collection<int, array{
     *         id: int,
     *         team_id: int|null,
     *         name: string,
     *         owner: string|null,
     *         days: array<int, string>,
     *     }>,
     *     footer: string,
     * }
     */
    public function build(): array
    {
        return [];
    }
}
