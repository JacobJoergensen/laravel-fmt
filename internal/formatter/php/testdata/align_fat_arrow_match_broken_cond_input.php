<?php

$label = match (true) {
    $x === 1 => 'a',
    $xx === 22 => 'bb',
    $normalizedSubscriptionPlanIdentifier
        === \App\Billing\Plans\SubscriptionPlan::EnterpriseAnnualCommitment => 'enterprise',
    $normalizedSubscriptionPlanIdentifier
        === \App\Billing\Plans\SubscriptionPlan::ProfessionalMonthlyTier => 'professional',
    $y === 3 => 'c',
    $yy === 44 => 'dd',
    default => 'other',
};
