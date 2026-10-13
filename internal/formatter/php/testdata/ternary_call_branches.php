<?php

$redirectRoute = $account->type === AccountType::Paid
    ? route('projects.reports.summaries', $this->contextProject)
    : route('projects.settings', ['project' => $this->contextProject, 'tab' => 'team']);
$nested = $contextProject
    ? ($formType === AccountType::Paid
        ? route('projects.reports.summaries', $contextProject)
        : route('projects.settings', ['project' => $contextProject, 'tab' => 'team']))
    : null;
$tag = $thisIsAFairlyLongConditionExpressionForTesting
    ? buildTheResponseObject('some.route.name', $someArgumentValue)
    : null;
