<?php

return $this->candidateRanges->containsMatch(
    static fn (CandidateRange $range): bool => $range->includesOffset($searchOffset),
);
