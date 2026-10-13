<?php

$config = [
    'options' => extension_loaded('pdo_mysql') ? array_filter([Pdo::ATTR_TIMEOUT => env('DB_TIMEOUT')]) : [],
];

$payload = $verbose ? [
    'status' => 'ok',
    'detail' => $message,
] : [];
