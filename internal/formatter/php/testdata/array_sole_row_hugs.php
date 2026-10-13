<?php

Agent::fake(Classifier::class, [['items' => [['record_id' => $record->id, 'category' => Category::Widgets->value, 'name' => 'BLUE WIDGET', 'quantity' => 2]]]]);

$rows = [['id' => 1, 'name' => 'first']];

DB::table('settings')->insert([[
    'key' => 'theme',
    'value' => 'dark',
]]);

$authored = [
    ['id' => 1, 'name' => 'first row that the author placed on its own line'],
];

$commented = [/* the only row */ ['id' => 1, 'name' => 'a row long enough that the array has to break across lines']];

$table = [['id' => 1, 'name' => 'first'], ['id' => 2, 'name' => 'second'], ['id' => 3, 'name' => 'third']];

function anchors(int $commentId, string $quote): Collection
{
    return collect([new CommentAnchor(['comment_id' => $commentId, 'token_path' => $tokenPath, 'quoted_text' => $quote])]);
}
