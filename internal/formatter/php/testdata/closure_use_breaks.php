<?php

$this->withProgressBar($subscribers, function (NewsletterSubscriber $subscriber) use (
    $isDryRun,
    &$synced,
    &$skipped,
    &$failed,
): void {
    $synced++;
});
