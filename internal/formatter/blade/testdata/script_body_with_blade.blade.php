<div>
<script>
const config = @json($config);
const count = {{ $count }};
const label = `Items: {{ $count }}
  on two lines`;
@if ($debug)
    console.log(config);
@endif
</script>
</div>
