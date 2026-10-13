<?php

foreach ($lines as $line) {
    if (
        $date === null
        && preg_match('/^(?:\d{4}-\d{2}-\d{2}|\d{1,2}[\/.-]\d{1,2}[\/.-]\d{2,4}|[A-Za-z]+ \d{1,2},? \d{4})$/', $line)
            === 1
    ) {
        $date = $line;
    }
}
