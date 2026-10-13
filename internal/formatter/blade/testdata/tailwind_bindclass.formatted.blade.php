<div>
    <x-a :class="$active ? 'bg-blue-500 p-4 text-white' : 'p-4 text-gray-500'">ternary</x-a>
    <x-b :class="$dragOver && 'bg-primary-50 ring-2 ring-primary-500 ring-inset'">short-circuit</x-b>
    <x-c :class="[$color, $isIdle ? 'p-4 text-white' : 'flex p-4']">array</x-c>
    <x-d :class="'base flex '.($x ? 'bg-red-500 p-4' : '')">concat keeps edge space</x-d>
    <x-e :class="$type === 'admin p-4' ? 'flex text-white' : ''">condition string not sorted</x-e>
    <x-f :class="colorFor($id)">function call untouched</x-f>
    <span :class="active ? 'text-white bg-blue-500 p-4' : ''">plain element is Alpine, left as written</span>
</div>
