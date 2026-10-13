<?php

function inline(#[Required] int $id): void {}

function wrapped(
    #[Required]
    LongParameterTypeName $first,
    AnotherLongParameterType $second,
    #[Validated]
    YetAnotherTypeName $third,
): void {}
