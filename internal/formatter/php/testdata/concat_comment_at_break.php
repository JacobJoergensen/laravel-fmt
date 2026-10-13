<?php

$rows = rowsFrom(
    "name,email,role\n".
    "First,first@example.com,Owner\n".  // trailing its row
    "Second,second@example.com,Guest\n"
);

$label = 'alpha'.
    // on its own line
    'beta';
