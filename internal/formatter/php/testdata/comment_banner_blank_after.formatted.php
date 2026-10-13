<?php

return [

    /*
    |--------------------------------------------------------------------------
    | Default Disk
    |--------------------------------------------------------------------------
    |
    | Here you may specify the default filesystem disk.
    |
    */

    'default' => env('FILESYSTEM_DISK', 'local'),

    // a short banner

    'threads' => 4,

    'welded' => true,

    'disks' => [
        'local' => [
            'driver' => 'local',
        ],

        // 's3' => [
        //     'driver' => 's3',
        // ],
    ],

];

function boot(): void
{
    // a banner

    $registry = resolve(Registry::class);

    /*
    | block banner
    */

    $registry->boot();
}
