@foreach ($items as $item)
    <li>
        @if ($item->active)
            {{ $item->name }}
        @endif
    </li>
@endforeach
