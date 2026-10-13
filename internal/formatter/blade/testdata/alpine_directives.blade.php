<button x-on:keydown.enter.prevent="submit()" x-model.number.debounce.500ms="qty">Go</button>
<div x-transition:leave.scale.90 x-transition:enter.duration.500ms></div>
<div
    x-data="{
        open: false,
        toggle() {
            this.open = !this.open
        }
    }"
>
    content
</div>
