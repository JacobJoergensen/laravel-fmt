<?php

$a = new static::$classes[Foo::class]($x);
$b = new self::$default($x);
$c = new $obj->prop($x);
$d = new $obj->a->b($x);
$e = new $arr[0]($x);
$f = new $obj::$prop($x);
$g = new $factory->resolver()($x);
