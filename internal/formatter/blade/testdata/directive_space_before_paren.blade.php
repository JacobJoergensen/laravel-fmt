@if ($count > 0)
    @foreach ($items as $item)
        <li>row</li>
    @endforeach
@elseif ($count === 0)
    <p>none</p>
@endif
