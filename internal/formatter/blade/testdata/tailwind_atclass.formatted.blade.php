<div>
    <div @class(['bg-blue-500 p-4 text-white', 'text-sm font-bold' => $active, "text-{$c}" => true, $dynamic])>x</div>
    @class(['bg-blue-500 p-4 text-white', 'mt-2 flex' => $cond])
    <x-badge :class="'bg-blue-500 p-4 text-white'">bound sorts too</x-badge>
</div>
