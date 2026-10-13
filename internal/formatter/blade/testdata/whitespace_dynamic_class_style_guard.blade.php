<section>
<div style="white-space: {{ $mode }}"><b>Echoed value</b>
    indented line kept as written</div>
<div class="{{ $notes ? 'whitespace-pre-line' : '' }}"><b>Echoed class</b>
    indented line kept as written</div>
<div :class="{ 'whitespace-pre-wrap': $wrap }"><b>Bound class</b>
    indented line kept as written</div>
<div @class(['whitespace-pre-line' => $notes])><b>Class directive</b>
    indented line kept as written</div>
<div @style(['white-space: pre-line' => $notes])><b>Style directive</b>
    indented line kept as written</div>
<div @class(['p-4' => $padded]) style="color: {{ $color }}"><b>Unrelated</b>
    line reflowed</div>
</section>
