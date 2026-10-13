<?php

class A {
    public function b() {
        $fooBarBaz = $this->alpha->betas->flatMap(
            fn (Beta $beta) => $beta->gammasAsSubjectFooBarBazQuxQuuxCorgeG->map(function (Gamma $gamma) use ($beta): array {
                return [$gamma, $beta];
            }),
        )->values();
    }
}
