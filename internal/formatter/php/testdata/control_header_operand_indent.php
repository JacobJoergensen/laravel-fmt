<?php

class Policy
{
    public function check(string $ability, User $user): void
    {
        if (($ability === 'viewAny' || $ability === 'view') && $user->hasPermissionTo(Permissions::ViewAllInvoices->value)) {
            $isMember = $event->notifiable instanceof User && $event->notifiable->subscriptions()->where('id', $id)->exists();
        }
    }
}
