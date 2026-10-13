<span {{ $attributes }}>@foreach ($items as $item)@if ($item->ok){!! $item->html !!}@else<x-badge :item="$item" />@endif @endforeach</span>
