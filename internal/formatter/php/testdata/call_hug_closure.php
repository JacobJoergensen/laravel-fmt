<?php

$query->where(function ($q) use ($search): void {
    $q->where('title', 'like', sprintf('%%%s%%', $search));
});
