<?php

$this->post('/newsletter', [
    'email' => 'test@gmail.com',
    'honeypot' => '',
])
    ->assertStatus(400);
