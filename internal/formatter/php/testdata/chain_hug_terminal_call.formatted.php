<?php

$existing->forceFill([
    'role' => $role->value,
    'expires_at' => $expiresAt,
    'created_by' => $createdBy->id,
    'warehouse_location' => $warehouseLocation,
])->save();
