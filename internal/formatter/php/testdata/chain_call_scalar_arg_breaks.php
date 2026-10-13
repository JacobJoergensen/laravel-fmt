<?php

$query->where('project_name', 'like', sprintf('%%%s%%', $search))
    ->orWhere('project_logline', 'like', sprintf('%%%s%%', $search));
