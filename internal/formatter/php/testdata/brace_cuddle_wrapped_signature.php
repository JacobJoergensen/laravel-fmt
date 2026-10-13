<?php

function persistAggregate(
    FirstRepositoryDependency $first,
    SecondRepositoryDependency $second,
    ThirdRepositoryDependency $third,
): PersistResult
{
    return true;
}

function tally(int $a): int
{
    return $a;
}
