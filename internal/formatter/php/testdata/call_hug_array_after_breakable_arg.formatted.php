<?php

class ReportController
{
    public function show(Report $report): Response
    {
        Log::warning('Report rendering failed for '.$report->id, [
            'report' => $report->id,
        ]);

        $response = Http::withToken($token)->get('https://example.test/reports/'.$report->id, [
            'fields' => 'name,size',
        ]);

        $label = __($report->revision ? 'reports.subject_revision' : 'reports.subject', [
            'name' => $report->name,
        ]);

        return response(app(Renderer::class)->bytes($report), 200, [
            'Content-Type' => 'application/pdf',
        ]);
    }

    public function export(Report $report): string
    {
        $summary = sprintf(
            '%s: %s',
            [$report->id, $report->name],
            format(
                $report->total,
            ),
        );

        Log::info(
            implode(', ', [
                'first',
            ]),
            [
                'report' => $report->id,
            ],
        );

        return route(
            'reports.export.'.$report->format.'.with.a.much.longer.segment.that.pushes.the.head.past.the.budget',
            [
                'report' => $report->id,
            ],
        );
    }
}

planEntry(
    ['location_name' => 'WAREHOUSE', 'location_id' => $location->id, 'time_of_day' => 'DAY'],
    ['estimated_time' => 10.0],
);

$validator = Checker::make(['email' => $value], [
    'email' => ['required', 'email', 'max:255', 'unique:users,email', 'not_in:'.$reserved],
    'name' => ['required', 'string', 'max:120'],
]);

planEntry(['location_name' => 'WAREHOUSE', 'location_id' => $location->id, 'time_of_day' => 'DAY'], [
    'estimated_time' => 10.0,
]);
