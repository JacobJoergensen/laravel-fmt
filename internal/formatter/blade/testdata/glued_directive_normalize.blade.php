<div>
    <span>@lang("a") <b>x</b></span>
    <span>@lang("a")  <b>x</b></span>
    <span>{{$a}}@js(  [1,2] )<b>x</b></span>
    <span><b>@choice("m.x",  $n)</b> items</span>
    <a href="#">Hi@lang("a")!<b>x</b></a>
    <span>@csrf<b>x</b></span>
    <span>@if($a)<b>x</b>@endif</span>
    <span>@foreach($items as $i)<b>{{$i}}</b>@endforeach</span>
    <span>(@if($a) y @elseif($b) z @else w @endif)!</span>
    <div>@if($a)<b>x</b>@endif</div>
    <span>@include('partials.some.really.long.view.name', ['user' => $user, 'items' => $items->filter(fn ($item) => $item->isVisible() && $item->belongsTo($user))->values(), 'mode'  =>  'compact'])<textarea>x</textarea></span>
</div>
