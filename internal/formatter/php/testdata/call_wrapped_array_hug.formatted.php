<?php

foo(bar('a'), baz([
    'alpha' => 1,
]));

foo($x->bar(), baz([
    'alpha' => 1,
]));

foo($x->bar('a'), baz([
    'alpha' => 1,
]));

foo($x->bar('a'), [
    'alpha' => 1,
]);
