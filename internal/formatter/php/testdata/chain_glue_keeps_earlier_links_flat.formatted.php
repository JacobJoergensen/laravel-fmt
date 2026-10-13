<?php

run(function () {
    f(1 % g('')(''))->a(1)->summarizeEveryEntryOfTheReportingPeriodForTheSelectedWorkspaces(Workspace::ID, windowBetween(
        '2026-1',
        '2026-2',
    ));
});
