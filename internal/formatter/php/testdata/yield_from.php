<?php

function gen()
{
    yield from $source;
    yield from [1, 2, 3];
}
