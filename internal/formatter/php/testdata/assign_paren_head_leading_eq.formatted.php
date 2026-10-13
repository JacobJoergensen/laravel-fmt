<?php

function f()
{
    foreach ($rows as $row) {
        $totals[$row->group_id]['count_per_slot'][$slotKey]
            = ($totals[$row->group_id]['count_per_slot'][$slotKey] ?? 0) + (float) $row->total_count;
        $totals[(int) $row->group_identifier]['bytes'] = ($totals[(int) $row->group_identifier]['bytes'] ?? 0)
            + (int) $row->bytes;
        $totals[(int) $row->group_identifier_with_long_name]['bytes']
            = ($totals[(int) $row->group_identifier_with_long_name]['bytes'] ?? 0) + (int) $row->bytes;
        $short
            = 1;
        $this->first
            = $this->second
            = $this->third
            = null;
        $buffer[$key]
            .= $suffix;
    }
}
