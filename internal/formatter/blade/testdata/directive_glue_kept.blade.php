<p>{{ $name }}@csrf</p>
@unknown{{ $value }}
{{ $before }}@if ($ready)
    <span>ready</span>
@endif{{ $after }}
@php
    $count = 1;
@endphp{{ $count }}
0<x-a class="b">{{ $glued
}}</x-a>
@csrf
@method('PUT')
