<?php

$normalized = is_array($row['values'] ?? null) ? array_values(array_filter(array_map('strval', $row['values']))) : [];
