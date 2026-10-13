<?php

$value = $this->foo('x')
    ->bar()
    ->baz;

return Foo::bar('x')
    ->baz()
    ->qux;

$value = Foo::bar('x')
    ->baz()
    ->qux;

Foo::bar('x')
    ->baz()
    ->qux;

$value = Foo::bar('x')
    ->baz()
    ->qux()
    ->quux;
