<?php

expect(
    Order::where('status', 'shipped')->whereIn('region', ['north', 'south', 'east', 'west', 'central'])->count(),
)->toBe(3);
