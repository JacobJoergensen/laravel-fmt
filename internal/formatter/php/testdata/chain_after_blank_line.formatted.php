<?php

DB::table('first')->where('id', $id)->delete();
DB::table('second')->where('id', $id)->delete();

DB::table('third')->where('id', $id)->delete();

$model->forceFill([
    'a' => 1,
    'b' => 2,
])->save();
