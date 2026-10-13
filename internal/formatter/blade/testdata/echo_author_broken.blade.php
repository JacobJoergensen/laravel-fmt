<div>
    <div class="footnote">
        {!!
            __('forms.pdf.table_footnote', [
                'result' => '<strong>'.e($result).'</strong>',
                'date'   => e($generatedAt->translatedFormat('Y-m-d H:i')),
            ])
        !!}
    </div>
    <p>{{ __('report.sales_totals.meta', [
        'start' => $report->period_start->translatedFormat('M j'),
    ]) }}</p>
    <a href="{{ route('questions.show', [
        'question' => $question->id,
    ]) }}">link</a>
    <x-select
        :options="collect([
            'draft' => __('Draft'),
            'final' => __('Final'),
        ])->all()"
        name="status"
    />
    <p>{{
        $user->name
    }}</p>
    <p>{{ __('This is a really long translation string that should never be exploded across multiple lines by the formatter') }}</p>
    <p>{{ $user->posts()
        ->where('published', true)
        ->count() }}</p>
</div>
