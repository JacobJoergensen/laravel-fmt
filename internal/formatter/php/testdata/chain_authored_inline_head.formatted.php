<?php

$label = $invoice->issued_at->copy()->setTimezone($account->timezone())->format('M j, Y');
