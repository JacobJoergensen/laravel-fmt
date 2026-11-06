<div x-data="{
      step: {{ $start }},
          label: '{{ $label }}',
        open() {
    this.show({{
        $id
    }})
          },
}">
    <span x-data="{
        raw: {!! $config !!},
            ready: false,
    }"></span>
    <p x-init="
  {{ $hook }}();
          $nextTick(() => {
      ready = true
  })
  "></p>
</div>
