<?php

$id = namespace\Config::VERSION;
namespace\Helpers\boot();
$event = new namespace\Events\Started();

function resolve(namespace\Contracts\Handler $handler): namespace\Result
{
    return $handler->run();
}
