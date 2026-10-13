<p>{{ $a /* c1 */, $b }}</p>
<p>{!! $a /* c2 */, $b !!}</p>
<p>{{ $v /* c3 */, }}</p>
@if($a /* c4 */, $b)
    @include("x" /* c5 */, ["a" => 1])
@endif
@php
    $y = [1 /* c8 */, 2];
@endphp
@php($x = [1 /* c6 */, 2])
<x-a :items="[1 /* c7 */, 2]" />
<p>{{ $plain,$list }}</p>
