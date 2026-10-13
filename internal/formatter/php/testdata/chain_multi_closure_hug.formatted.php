<?php

return $query->where(static function (Builder $q): void {
    $q->whereNull('archived_at')->orWhere('archived_at', '>', now());
})->where(static function (Builder $q): void {
    $q->whereNull('quota')->orWhereColumn('used', '<', 'quota');
});
