<div>
    <div @class(['text-white bg-blue-500 p-4', 'font-bold text-sm' => $active, "text-{$c}" => true, $dynamic])>x</div>
    @class(['p-4 bg-blue-500 text-white', 'mt-2 flex' => $cond])
    <x-badge :class="'text-white bg-blue-500 p-4'">bound sorts too</x-badge>
</div>
