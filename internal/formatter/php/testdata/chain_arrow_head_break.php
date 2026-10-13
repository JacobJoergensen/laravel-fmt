<?php

expect(fn () => resolve(ReportBuilder::class)->build($schedule->refresh()->entries->first(), 'weekly', 'pdf'))->toThrow(RuntimeException::class, 'no entries to report');
