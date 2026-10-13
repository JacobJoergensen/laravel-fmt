<?php

$count = Model::query()
    ->where('group_id', $group->id)
    ->whereHas('items', static fn ($q) => $q->where('items.parent_id', $parent->id)->where('items.active', false))
    ->count();
