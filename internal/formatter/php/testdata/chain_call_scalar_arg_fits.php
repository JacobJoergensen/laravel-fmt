<?php

$q->where('name', sprintf('%s', $s))->orWhere('logline', sprintf('%s', $s));
