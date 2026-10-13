<?php

if (
    $user->isAdministrator() || $user->ownsResource($resource) || $user->hasPermissionTo(Permission::EditContent->value)
) {
    return true;
}
