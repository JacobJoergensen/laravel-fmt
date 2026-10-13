<?php

$sub->where('x', 1)->where(function ($i): void {
    $i->foo();
});
