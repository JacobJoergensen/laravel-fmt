<?php

it('sends', function () {
    Notification::assertSentOnDemand(OrderConfirmed::class, fn (OrderConfirmed $notification): bool => $notification->submission->is($submission));

    $this->mock(PdfExporter::class, fn ($mock) => $mock->shouldReceive('bytes')->andReturn(file_get_contents(base_path('tests/Fixtures/Sales/invoices.pdf'))));

    $query->orWhereHas('projects.users', static fn (\Illuminate\Contracts\Database\Query\Builder $u) => $u->where('users.id', $user->id));

    $users = $items->map(fn (Item $item) => User::query()->create(['name' => $item->name, 'email' => $item->email, 'team' => $item->team]));

    $short = $items->map(fn ($i) => $i->id);

    Route::middleware('auth')->group(fn () => require base_path('routes/admin.php'));
});
