<p>{{ $value,false }}</p>
<p>{!! $first ,  $second !!}</p>
<p>{{{ $a, $b }}}</p>
<p>{{ $label, doubleEncode: false }}</p>
<p>{{ ...$parts }}</p>
<p>{{ $trailing, }}</p>
<p>{{ $noted /* kept */ }}</p>
<p>{{ $first, /* between */ $second }}</p>
<div class="{{ $class, false }}">{{ $x,$y }}</div>
@if ($ok)
    {{ $value , false }}
@endif
<p>{{ $line // kept
}}</p>
