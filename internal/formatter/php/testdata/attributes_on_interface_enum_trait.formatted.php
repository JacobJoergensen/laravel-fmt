<?php

#[Bindable(Concrete::class, static function () {
    return true;
})]
#[Marker]
interface Contract {}

#[Flagged]
enum Status: string
{
    case Active = 'active';
}

#[Mixin]
trait Shareable {}
