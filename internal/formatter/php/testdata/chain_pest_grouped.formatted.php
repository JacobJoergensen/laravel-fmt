<?php

expect($content)
    ->toContain('<rss version="2.0"')
    ->and($content)
    ->toContain('<channel>')
    ->and($content)
    ->toContain('<link>'.url('/blog').'</link>')
    ->and($content)
    ->toContain(sprintf('<title>%s</title>', $this->post->title))
    ->and($content)
    ->toContain('<link>'.url('/blog/'.$this->post->slug).'</link>');
