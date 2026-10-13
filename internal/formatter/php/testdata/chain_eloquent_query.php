<?php

$subscribers = NewsletterSubscriber::query()->whereNotNull('email_verified_at')->oldest('email_verified_at')->get();
