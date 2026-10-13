<?php

class TaskRepository
{
    public function add(array $data, int $groupNumber): Task
    {
        $task = $taskList
            ->tasks()
            ->create(array_merge($data, [
                'group_number' => $groupNumber,
                'task_number' => $taskNumber,
                'sort_order' => $sortOrder,
                'status_label' => $statusLabel,
            ]));

        $lines = $csv
            |> trim(...)
            |> strtoupper(...)
            |> array_filter(...);

        $single = $csv |> trim(...);

        $fileA->addMedia($upload)->toMediaCollection('file');

        $users = $ids->isNotEmpty() ? User::query()->whereIn('id', $ids)->get() : collect();

        return match ($subscription->type) {
            SubscriptionType::Direct => $subscription->participants()
                ->where('users.id', $user->id)
                ->exists(),
            SubscriptionType::Group => $subscription->isGroupAdmin($user),
            default => false,
        };
    }
}
