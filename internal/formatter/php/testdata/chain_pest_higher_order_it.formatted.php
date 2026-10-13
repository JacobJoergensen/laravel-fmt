<?php

it('redirects guests to login')->get('/dashboard')->assertRedirect('/login');
