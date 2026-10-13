<?php

final class Cue
{
    public static function isContinuation(string $cue): bool
    {
        return preg_match(
            '/\b(?:CONTD|CONTINUED|CONTINUING|FORTSAT|FORTS|FORTSETTELSE|FORTSATTNING|FORTSETZUNG|VERVOLG|SUITE)\b/ui',
            $cue,
        ) === 1;
    }

    public function guard($alpha, $bravo): bool
    {
        return isFoo($alpha)
            && isBarBazQuxWithAnAmpleNameToWrapTheLine($bravo, $alpha, $bravo, $alpha, $bravo, $alpha, $bravo);
    }
}
