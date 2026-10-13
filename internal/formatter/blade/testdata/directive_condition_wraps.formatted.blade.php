<div>
    @if (
        $statusOption === $taskList->status
        || ($statusOption === TaskListStatus::Locked && $this->canLock)
        || $this->isOwner
    )
        <p>editable</p>
    @elseif ($statusOption === TaskListStatus::Archived && $this->canRestore && $this->hasPermission('restore'))
        <p>restorable</p>
    @endif

    @if ($ready)
        <p>short conditions stay flat</p>
    @endif

    @if ($object->someVeryLongPropertyChain->thatCannotBreakAnywhereAtAll->becauseItIsOnePath->reallyTrulyNoBreakPoints)
        <p>nothing to break, so it stays put</p>
    @endif

    <div>
        <div>
            <div>
                <div>
                    <div>
                        <div>
                            @if ($item->status === ItemStatus::Done)
                                <p>done</p>
                            @elseif (
                                $item->status === \App\Enums\Checklist\ItemStatus::InProgressWithReviewPendingApproval
                            )
                                <p>fits on its own line, so it moves under the opener</p>
                            @endif
                            <span>
                                {{ $label }}
                                @if (
                                    $this->activeNotification->type !== \App\Enums\Notification\NotificationType::Direct
                                )
                                    · {{ $count }}
                                @endif
                            </span>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>

<div>
    @if (
        registerTheHandlerForTheCurrentWorkspaceAndEveryOneOfItsMembersWhoseAccessIsStillPendingReviewByAnAdministrator: resolve#
        ()
    )
        x
    @endif
</div>
