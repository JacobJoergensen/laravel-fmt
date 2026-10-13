<?php

it('checks the notice', function (): void {
    Notification::assertSentOnDemand(
        AddressChanged::class,
        fn (
            AddressChanged $notification,
            array $channels,
            AnonymousRecipient $recipient,
        ): bool => $recipient->routes['mail'] === 'old@example.com'
            && $notification->replacedAddress === 'old@example.com',
    );

    Notification::assertSentTo(
        $user,
        fn (Notice $notice, array $channels): bool => $notice->subject === 'A subject line'
            && $notice->body === 'A body that runs on for a good while past the print width',
    );

    $names = $rows->map(fn (Row $row): string => $row->firstName.' '.$row->lastName.' ('.$row->department->displayName
        .') since '.$row->startedAt);

    Notification::assertSentOnDemand(
        Notice::class,
        fn (
            Notice $notice,
            array $channels,
        ): bool => $notice->ok,
    );

    run(
        $item,
        fn (
            Notice $notice,
        ): bool => $notice->ok,
    );
});
