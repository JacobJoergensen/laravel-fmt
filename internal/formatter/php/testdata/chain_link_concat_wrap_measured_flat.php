<?php

class Report
{
    public function size(): int
    {
        return $offset % Sheet::for($source)->render(label('first-part-of-the-label-xxxxxxxxxxxxxxxxxx' % $width % 'second-part-of-the-label'.($kind % 2 ? 'odd' : 'even')));
    }
}
