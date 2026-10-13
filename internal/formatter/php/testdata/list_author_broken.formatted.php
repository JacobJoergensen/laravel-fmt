<?php

final class Notice
{
    public function __construct(
        public string $subject,
        public bool $urgent = false,
    ) {}
}

final class Report
{
    public function __construct(string $title, int $pages) {}

    public function render(
        string $title,
        int $pages,
    ): string {
        $url = Url::signed(
            'report.show',
            $this->expiry(),
            ['id' => $this->id],
        );

        $item = new Entry(
            $title,
            $pages,
        );

        $flat = sprintf('%s: %d', $title, $pages);

        $wide = $this->summaries->describe(
            $title,
            $pages,
            $this->expiry(),
            $this->audience,
            $this->locale,
            $this->timezone,
            $url,
        );

        Log::info('rendered', [
            'title' => $title,
        ]);

        return match ($this->kind) {
            Kind::Draft,
            Kind::Review => 'open',
            Kind::Final, Kind::Archived => 'closed',
        };
    }
}
