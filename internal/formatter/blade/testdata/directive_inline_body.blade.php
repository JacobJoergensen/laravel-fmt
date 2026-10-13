@if ($previous && $current && $previous->id !== $current->id)
    Tagged {{ $previous->label }} (v{{ $previous->version }}); item is now on {{ $current->label }} (v{{ $current->version }}).
@endif
