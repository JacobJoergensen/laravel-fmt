<script>
  const settings = @json($settings);
      const limit = {{ $limit }};
        const empty = '@lang('items.empty')';
  function load(page) {
        return fetch('/items?page=' + page, {
    limit: limit,
            });
  }
</script>

<script>
    @if ($debug)
        console.log('debug');
    @endif
      start();
</script>

<script>
  const note = `first
      second`;
      run(note);
</script>

<script>
      /**
         * Loads a page.
   */
  function load(page) {
          /** Keeps
     * its shape. */ return page;
  }
</script>
