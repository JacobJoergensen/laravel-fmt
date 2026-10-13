<div class="a">
    <div class="b">
        {{ $a }} ·
        <x-foo :value="$b" format="c" />
    </div>
    <p>
        <strong>{{ $item['a'] }}</strong>
        · {{ $item['b'] }}
    </p>
    <div class="b">
        {{ $a }}
        · {{ $b }}
    </div>
    <div class="b">{{ $a }} · <span>x</span></div>
</div>
