<button wire:click="increment">+</button>
<input wire:model="name">
<input wire:model="form.name">
<button wire:click="add( 1 , 2 )">Add</button>
<form wire:submit.prevent="save">Save</form>
<button wire:click="select({{ $id }})">Pick</button>
<button x-on:click="showMenu = ! showMenu">Toggle</button>
