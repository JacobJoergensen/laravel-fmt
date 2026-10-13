<?php

it('builds the export archive and keeps the blank lines that separate each setup step from the assertions in the test body', function (): void {
    $first = makeFirst();

    $second = makeSecond();
    configure($first, $second);

    run($first);
});
