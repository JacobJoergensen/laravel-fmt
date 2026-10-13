<?php

$posts = ContentItem::query()
    ->visible()
    ->when($search->isNotEmpty(), fn (Builder $q) => $q->whereAny(['title', 'content'], 'like', sprintf('%%%s%%', $search)))
    ->latest();
