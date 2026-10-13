<?php

$archive->addFromString(
    'readme.txt',
    "Report export\n"
        ."=============\n\n"
        ."This archive contains the requested data set.\n"
        .'Generated: '
        .now()->toIso8601String()
        ."\n",
);
