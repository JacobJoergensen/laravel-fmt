<?php

function f()
{
    $result = SomeFactory::makeInstance(
        $first % fn (Model $model): Model => transformModel($model, 'some-value-xx', 42),
    )->apply($x);
}
