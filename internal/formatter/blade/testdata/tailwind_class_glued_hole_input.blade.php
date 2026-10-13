<div class="rounded-full ring-offset-2 bg-{{ $color }}-500">trailing prefix</div>
<div class="p-4 flex bg-{{ $c }}-500 m-2">prefix mid-list</div>
<div class="rounded-full ring-offset-2 text-{{ $tone }}">prefix at end</div>
<div class="text-white bg-blue-500 {{ $extra }}">space before hole still sorts</div>
<div class="{{ $base }} text-white bg-blue-500">space after hole still sorts</div>
<div class="status-{{ $state }} p-4 flex">suffix hole leads</div>
<div @class(['rounded-full ring-offset-2 bg-'.$color.'-500'])>concat prefix</div>
<div @class(['base flex '.($x ? 'p-4 bg-red-500' : '')])>concat edge space still sorts</div>
<div @class(['p-4 '.$mid.' flex bg-'.$c.'-500'])>concat both sides</div>
<div @class(['size-5 rounded-full ring-offset-2 transition bg-'.$color.'-500', 'ring-2' => $on])>concat with condition</div>
