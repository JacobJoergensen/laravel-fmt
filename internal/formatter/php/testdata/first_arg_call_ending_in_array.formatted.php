<?php

function foo()
{
    Foo::toast(__('travel.per_diem.saved_successfully_for_everyone', [
        'count' => $this->rows->count(),
        'x' => 1,
    ]), variant: 'success');
    Foo::toast(__('travel.per_diem.generated', [
        'created' => $result['created'],
        'updated' => $result['updated'],
    ]), variant: 'success');
    Foo::toast(
        foo('travel.per_diem.saved_successfully_for_everyone_and_more_text', $this->rows->count(), $xxxx),
        'success',
    );
}
