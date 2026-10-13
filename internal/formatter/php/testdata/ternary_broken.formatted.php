<?php

$totals = $useCache
    ? DB::table('records')
        ->join('groups', 'records.group_id', '=', 'groups.id')
        ->where('groups.region_id', $regionId)
        ->pluck('amount', 'group_id')
        ->all()
    : [];
$label = $enabled
    ? 'This feature is currently enabled for every signed-in account'
    : 'This feature is disabled for everyone at the moment';
