<?php

function foo()
{
    $x = r(fn ($c) => $c === Foo::aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
        || $c === Foo::bbbbb);
    $y = rejectxxxxxxxxxx(fn ($c) => $c === Foo::aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
        || $c === Foo::bbbbb);
}
