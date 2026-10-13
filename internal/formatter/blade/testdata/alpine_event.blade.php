<div
    @keydown.window.ctrl.1.prevent="$wire.setStatus('one')"
    @keydown.window.cmd.1.prevent="$wire.setStatus('one')"
    @keydown.window.ctrl.2.prevent="$wire.setStatus('two')"
    @keydown.window.cmd.2.prevent="$wire.setStatus('two')"
>
    <button @click="open = !open" class="btn">Toggle</button>
    <button @click="select({{ $id }})">Pick</button>
</div>
