<?php

/** @var array<string, string> */  // Aligned
/** @param string $name */         // with this one

/** @var int */ // Before a statement
$count = 1;

/* first */ /* second */ // third
$total = 2;

class Widget
{
    /** @var int */ // Before a member
    public $size;

    public function run()
    {
        /* setup */ // Before a statement in a body
        return 1;
    }

    public function idle()
    {
        /* nothing */ // Only comments in the body
    }
}

$done = true;
/* tail */ // After the last statement
