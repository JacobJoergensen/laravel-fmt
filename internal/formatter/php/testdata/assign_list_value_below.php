<?php

class Foo {
    public function bar(): void {
        [$aaaaaaaaaa, $bbbbbbbbb, $cccccccccccc, $ddddddddddd, $eeeeeeeeeeeee, $ffffffffffff, $gggggggggg, $hhhhhhh] =
            $this->foo([
                $this->a,
                $this->b,
            ]);
        [$aaaaaaaaaa, $bbbbbbbbb, $cccccccccccc, $ddddddddddd, $eeeeeeeeeeeee, $ffffffffffff, $gggggggggg, $hhhhhhh]
            = $this->foo([
                $this->a,
                $this->b,
            ]);
        [$aaaaaaaaaa, $bbbbbbbbb, $cccccccccccc, $ddddddddddd, $eeeeeeeeeeeee, $ffffffffffff, $gggggggggg, $hhhhhhh] = $this->foo($this->a, $this->b);
    }
}

function short(): void
{
    [$a, $b] = $this->foo($this->a, $this->b);
    [$aaaaaaaaaa, $bbbbbbbbb, $cccccccccccc, $ddddddddddd, $eeeeeeeeeeeee] = $this->fooWithLongerName($this->first, $this->second, $this->third);
}
