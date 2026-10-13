<section>
    @foreach ($rows as $row)
        @if ($group !== $last)
            @if ($last !== null)
                </ul>
            @endif
            <ul class="list">
        @endif
        <li>{{ $row }}</li>
    @endforeach
</section>
