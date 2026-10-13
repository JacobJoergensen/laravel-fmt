<?php

use App\Match\Resolver;
use League\Flysystem\ReadOnly\ReadOnlyFilesystemAdapter;

$adapter = new ReadOnlyFilesystemAdapter();
$config = \App\ReadOnly\Config::class;
$result = Domain\List\Collection::make();
