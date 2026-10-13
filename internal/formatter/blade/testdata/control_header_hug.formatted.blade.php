<div>
    @foreach ([
        'draft' => __('status.draft'),
        'review' => __('status.review'),
        'published' => __('status.published'),
    ] as $state => $label)
        {{ $label }}
    @endforeach
    @for (
        $index = 0, $total = count($this->someVeryLongCollectionNameThatIsLonger);
        $index < $total;
        $index += $this->stepSize
    )
        {{ $index }}
    @endfor
    @foreach ($this->members->filter(
        fn ($member) => $member->user_id !== null && $member->user_id !== $project->owner_id,
    ) as $member)
        {{ $member }}
    @endforeach
    @for (; ;)
        @break
    @endfor
    @if (in_array($status, [
        'pending',
        'processing',
        'awaiting_payment',
        'awaiting_shipment',
        'shipped',
        'delivered',
        'returned',
    ]))
        <p>open</p>
    @elseif (! Str::startsWith($request->path(), [
        'api/internal/webhooks',
        'api/internal/callbacks',
        'api/internal/xyz-extra',
    ]))
        <p>internal</p>
    @endif
    @unless (
        $user->isAdministrator()
        && $user->hasVerifiedEmail()
        && $request->routeIs('dashboard.*')
        && $request->xyzExtra()
    )
        <p>guest</p>
    @endunless
</div>
