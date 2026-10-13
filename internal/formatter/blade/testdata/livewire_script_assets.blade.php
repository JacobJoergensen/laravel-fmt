<div>
<p>{{ $count }}</p>
@script
<script>
const note = `first line
second line`;
$wire.on('saved', () => {
    alert(note);
});
</script>
@endscript
</div>
@assets
<script src="https://cdn.example.com/chart.js" defer></script>
<link rel="stylesheet" href="/chart.css">
@endassets
