<div class="mt-2 {{ $tone }} {!! $extra !!}">
    <span>short {{ $a }} {!! $b !!} @{{ $c }} @@ tail</span>
    <p>
        A deliberately long inline run so the container cannot sit on one line: <span>wrapped {{ $first }} then {!! $second !!} then a literal @{{ $third }} and an escaped at @@ end</span> trailing words to push past the print width.
    </p>
</div>
