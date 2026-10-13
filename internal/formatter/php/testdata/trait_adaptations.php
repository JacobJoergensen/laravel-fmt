<?php

class Sample
{
    use TraitA, TraitB {
        TraitA::hello insteadof TraitB;
        TraitB::hello as helloFromB;
        sayHello as protected;
        sayHello as public sayHelloPublicly;
        TraitA::run as private;
    }
}
