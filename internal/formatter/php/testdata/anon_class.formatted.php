<?php

$a = new class
{
    public $foo = 1;
};
$b = new class($x, $y)
{
    public function __construct($a, $b) {}
};
$c = new class extends Base
{
    public function run() {}
};
$d = new class implements Runnable, Serializable
{
    public function run() {}
};
$e = new class($ctx) extends Base implements Runnable
{
    public function run() {}
};
$f = new #[Deprecated] readonly class
{
    public $x;
};
$g = new class {};
