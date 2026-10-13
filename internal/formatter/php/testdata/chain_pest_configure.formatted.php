<?php

pest()
    ->extend(TestCase::class)
    ->use(RefreshDatabase::class)
    ->beforeEach(function (): void {
        Http::fake();
        Mail::fake();
        $this->withoutVite();
    })
    ->in('Feature');
