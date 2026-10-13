<div>
    <div
        x-data="presenceBoard({
        boardId: {{ (int) $this->board->id }},
        userName: @js(auth()->user()?->name ?? ''),
    })"
        class="x"
    >y</div>
    <div
        data-config="board({
        id: {{ $id }},
        name: 2,
    })"
    >z</div>
</div>
