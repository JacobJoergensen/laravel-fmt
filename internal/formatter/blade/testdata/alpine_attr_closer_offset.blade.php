<div>
    <div
        x-on:drop.prevent="
        dragOver = false
        $wire.moveItem(sourceKey, '{{ $dateKey }}')
    "
        x-on:dragstart="
        $event.dataTransfer.effectAllowed = 'move'
        $event.dataTransfer.dropEffect = 'move'
    "
        class="p-1"
    ></div>
</div>
