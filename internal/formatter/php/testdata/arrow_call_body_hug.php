<?php

return $query->get()->map(static fn (Row $row): string => sprintf('%s scored %d on %s', $row->name, $row->points, $row->date->format('Y-m-d')))->all();
