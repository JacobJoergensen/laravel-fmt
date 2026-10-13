<?php

function gen()
{
    yield;
    yield $value;
    yield $key => $value;
    yield 1 => 'one';
}
