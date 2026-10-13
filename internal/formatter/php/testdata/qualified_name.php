<?php

$a = App\Enums\Permissions::ViewProfiles->value;
$b = \App\Enums\Permissions::ViewProfiles->value;
$c = $user->can(\App\Enums\Permissions::ViewProfiles->value);
$d = $x instanceof \App\Models\Account;
$e = $x instanceof App\Models\Account;
$f = new \App\Models\Account();
$g = new App\Models\Thing($id);
$h = resolve(\App\Tenancy\TenantScopeResolver::class)->require()->id;
$i = empty($link['url']);
$j = ! empty($value);

try {
    doRisky();
} catch (\App\Exceptions\DomainError $e) {
    handle($e);
}
