<?php

expect($user->scopedRoles()->pluck('id')->all())->toEqual([$role->id]);
