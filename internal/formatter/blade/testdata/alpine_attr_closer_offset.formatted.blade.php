<div>
    <div
        x-mask:dynamic="
        dragOver = false
        $wire.moveItem(sourceKey, '{{ $dateKey }}')
    "
        x-mask:static="
        $event.dataTransfer.effectAllowed = 'move'
        $event.dataTransfer.dropEffect = 'move'
    "
        class="p-1"
    ></div>
</div>
