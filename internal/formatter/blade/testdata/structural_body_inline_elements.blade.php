<div>
    <strong>{{ __('report.weather') }}</strong> {{ $report->summary ?? '-' }}
    @if ($report->high)
        <span>({{ $report->high }})</span>
    @endif
</div>
<p>
    See <a href="/guide">the guide</a> for more.
    @if ($extra)
        Extra.
    @endif
</p>
<div>
    &bull; {{ __('common.generated_on') }} <x-datetime :value="now()" format="audit" />
    @if ($date)
        <x-datetime :value="$date" format="date" />
    @endif
</div>
<div>
    <strong>{{ __('report.weather') }}</strong>
    {{ $report->summary }}
    @if ($report->high)
        <span>y</span>
    @endif
</div>
<div>
    <svg class="size-4" viewBox="0 0 24 24">
        <path d="M4 6h16" />
    </svg> Label {{ $label }}
    @if ($extra)
        <span>y</span>
    @endif
</div>
