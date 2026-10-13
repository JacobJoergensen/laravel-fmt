<div>
    <span>x{{ $c->format('Y-m-d') }}</span>
    <span>EVENT DAY {{ $day->number }} @if ($day->date)- {{ $day->date->format('l, F j, Y') }}@endif</span>
</div>
