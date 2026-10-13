<?php

return $query->whereDate('availability_start', '<=', $end->format('Y-m-d'))
    ->whereDate('availability_end', '>=', $start->format('Y-m-d'));
