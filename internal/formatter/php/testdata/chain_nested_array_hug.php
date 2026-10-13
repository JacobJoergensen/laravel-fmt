<?php

class Seeder
{
    public function run(Account $account, Task $task, Task $next): void
    {
        $subscription = $account->subscriptions()->first()
            ?? $account->subscriptions()->forceCreate(['type' => 'default', 'reference' => 'sub_dev_'.$account->id, 'status' => 'active']);

        $task->subtasks->each(fn (Subtask $subtask) => $next->subtasks()->create(['title' => $subtask->title, 'position' => $subtask->position]));

        $task->subtasks->each(fn (Subtask $subtask) => $next->subtasks()->create(['title' => $subtask->title]));

        $task->subtasks->each(fn (Subtask $subtask) => $next->subtasksBelongingToTheCurrentAccountAndItsVeryLongRelationNameForTesting()->create(['title' => $subtask->title, 'position' => $subtask->position]));

        withoutGuard(fn () => Ledger::table('entries')
            ->where('id', $task->id)
            ->update(['event' => 'completed', 'completed_at' => now()]));
    }
}
