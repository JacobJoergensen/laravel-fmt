<?php

$rows = $groups->map(fn (Collection $group, int|string $groupId): object => (object) ['id' => (int) $groupId, 'name' => $names[$groupId] ?? '-', 'count' => $group->count()]);

$rows = $groups->map(fn (Collection $group, int|string $groupId): array => ['id' => (int) $groupId, 'name' => $names[$groupId] ?? '-', 'count' => $group->count()]);

$item = collect((object) ['id' => (int) $groupId, 'name' => $names[$groupId] ?? '-', 'count' => $group->count(), 'label' => $label]);

$counts = $groups->map(fn (Collection $group, int|string $groupId): int => (int) $group->filter(fn ($entry) => $entry->active)->count());

$empty = collect((object) []);

$emptyResultForTheCurrentReportingPeriod = $this->repository->createTheDefaultSummaryObjectForTheCurrentPeriod((object) []);
