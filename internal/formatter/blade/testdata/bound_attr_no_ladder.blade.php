<div>
    <div>
        <div>
            <div>
                <div>
                    <div>
                        <div>
                            <div>
                                <flux:icon :name="$this->attachmentIconForTheGivenMimeTypeValue($attachment->mime_type, $fallbackIcon)" class="size-5" />
                                <flux:badge :key="'row-picker-'.$row->identifier.'-'.$bucket->value.'-suffix'" class="text-xs">{{ $row->name }}</flux:badge>
                                <livewire:comment.input :context="['project_id' => $task->project_id, 'team_id' => $task->team_identifier, 'owner_id' => $task->owner_identifier]" />
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
