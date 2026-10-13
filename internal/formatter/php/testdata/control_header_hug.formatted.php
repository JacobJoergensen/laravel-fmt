<?php

foreach ([
    'draft' => __('status.draft'),
    'review' => __('status.review'),
    'published' => __('status.published'),
] as $state => $label) {
    echo $label;
}

foreach ($this->repository->findByOwner($owner, [
    'with' => 'relations',
    'order' => 'desc',
    'limit' => 100,
    'offset' => 5,
]) as $row) {
    echo $row;
}

if (in_array(
    $status,
    ['pending', 'processing', 'awaiting_payment', 'awaiting_shipment', 'shipped', 'delivered', 'returned'],
    true,
)) {
    return;
}

if (! Str::startsWith($request->path(), [
    'api/internal/webhooks',
    'api/internal/callbacks',
    'api/internal/health-checks',
])) {
    return;
}

while ($row = $statement->fetch(
    PDO::FETCH_ASSOC,
    PDO::FETCH_ORI_NEXT,
    $offsetValueThatIsLong + $moreValuesThatAreLonger,
)) {
    echo $row;
}

switch (match ($kind) {
    'a' => 'first-value-that-is-long',
    'b' => 'second-value-that-is-longer',
    'c' => 'third-value',
}) {
}

if (
    $user->isAdministrator()
    && $user->hasVerifiedEmail()
    && $request->routeIs('dashboard.*')
    && $request->isMethod('post')
) {
    return;
}

if (
    Container::getInstance()->make(Verifier::class)->verify([
        'value' => $value,
        'threshold' => $this->threshold,
        'x' => 1,
    ])
) {
    return;
}

if (
    in_array($status, [
        'pending',
        'processing',
        'awaiting_payment',
        'awaiting_shipment',
        'shipped',
        'delivered',
    ]) /* note */
) {
    return;
}

for (
    $index = 0, $total = count($this->someVeryLongCollectionNameThatIsLonger);
    $index < $total;
    $index += $this->stepSize
) {
    echo $index;
}

for (; ;) {
    break;
}

for ($i = 0; $i < 3;) {
    $i++;
}

foreach ($this->members->filter(
    fn ($member) => $member->user_id !== null && $member->user_id !== $project->owner_id,
) as $member) {
    echo $member;
}

foreach (
    $this->repository->findEverythingForTheCurrentInstallation($someVeryLongVariableNameThatIsLongerStill) as $row
) {
    echo $row;
}

if ($collection->filter()->each(function ($item) {
    return $item->valid();
})) {
    $valid = true;
}
