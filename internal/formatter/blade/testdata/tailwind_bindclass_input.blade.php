<div>
    <x-a :class="$active ? 'text-white bg-blue-500 p-4' : 'p-4 text-gray-500'">ternary</x-a>
    <x-b :class="$dragOver && 'ring-2 ring-primary-500 ring-inset bg-primary-50'">short-circuit</x-b>
    <x-c :class="[$color, $isIdle ? 'text-white p-4' : 'p-4 flex']">array</x-c>
    <x-d :class="'base flex ' . ($x ? 'p-4 bg-red-500' : '')">concat keeps edge space</x-d>
    <x-e :class="$type === 'admin p-4' ? 'text-white flex' : ''">condition string not sorted</x-e>
    <x-f :class="colorFor($id)">function call untouched</x-f>
    <span :class="active ? 'text-white bg-blue-500 p-4' : ''">plain element is Alpine, left as written</span>
</div>
