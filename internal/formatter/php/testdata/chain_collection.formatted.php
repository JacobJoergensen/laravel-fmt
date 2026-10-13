<?php

$active = $users->map(fn ($u) => $u->profile)->filter(fn ($p) => $p->isActive())->values();
