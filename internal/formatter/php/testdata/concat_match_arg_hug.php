<?php

function foo()
{
    $label = __('notifications.recipient.'.match ($recipient->kind) {
        RecipientKind::Contact => 'contact',
        RecipientKind::User => 'user',
    });

    $label = __('notifications.recipient.'.$recipient->kind->value.'.label_for_the_notification_settings_page_heading');
}
