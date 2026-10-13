<?php

$color = match ($day) {
    self::Working => '#4CAF50', // green for working days
    self::Hold    => '#FFC107', // amber for held days
    self::Dropped => '#E0E0E0', // grey for dropped days
    default       => '#000000', // black fallback
};
