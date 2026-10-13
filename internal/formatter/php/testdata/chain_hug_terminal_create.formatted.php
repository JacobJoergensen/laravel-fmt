<?php

$this->records()->create([
    'type' => $type,
    'created_at' => $timestamp,
    'position' => array_search($type, RecordType::cases(), true),
]);
