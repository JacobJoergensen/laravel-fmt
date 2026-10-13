<?php

$providers = [
    AppServiceProvider::class,
];

$statuses = [
    Status::ACTIVE,
];

$defaults = [
    Config::defaults(),
];

$handlers = [
    new Handler(),
];

$roles = [
    User::class => 'admin',
];

$flat = [Role::class];
