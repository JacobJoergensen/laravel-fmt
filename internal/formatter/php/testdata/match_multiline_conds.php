<?php

$label = match ($category) {
    Category::Electronics,
    Category::Furniture,
    Category::Clothing,
    Category::Books,
    Category::Toys,
    Category::Garden => 'physical',
    default => 'other',
};
