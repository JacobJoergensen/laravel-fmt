<?php

Schedule::command('reports:daily')
    ->dailyAt('02:00')
    ->withoutOverlapping()
    ->appendOutputTo($logPath);

Schedule::command('temp:clean')
    ->hourly()
    ->withoutOverlapping()
    ->appendOutputTo($logPath);

$total = User::query()->where('active', true)->count();
