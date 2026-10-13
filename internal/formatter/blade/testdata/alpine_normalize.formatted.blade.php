<div x-data="{ open: false, toggle() { this.open = !this.open } }">
    <button @click="open = !open;">Toggle</button>
    <button @click.prevent="$dispatch('picked', { id: 1 })">Pick</button>
    <span x-show="open && count > 0">a</span>
    <span x-text="open ? 'Hide' : 'Show'">b</span>
    <input x-bind:class="{ 'is-active': open }">
    <input x-model.debounce.500ms="query">
</div>
