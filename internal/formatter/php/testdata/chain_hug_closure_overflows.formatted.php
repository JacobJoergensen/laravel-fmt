<?php

$query = ScheduleEntry::query()
    ->with(['user', 'team', 'project', 'location'])
    ->where(function ($q) use ($rangeStart, $rangeEnd): void {
        $q->whereBetween('start_at', [$rangeStart, $rangeEnd]);
    });
