<?php

it('moves a lone call down rather than exploding its short array', function (): void {
    Livewire::test('pages::widgets.edit', ['widget' => $widget->id])
        ->set('colors', [Color::Red->value, Color::Blue->value]);

    expect($widget->refresh()->labels()->pluck('label')->sort()->values()->all())
        ->toEqual(['FIRST LABEL', 'SECOND LABEL']);

    expect($names)->toBe(['one', 'two']);

    $user = User::factory()->create([
        'name' => 'Example',
    ]);

    $item = Item::factory()->create([
        'group_id' => $group->id,
        'item_number' => '1000',
        'name' => 'Sample Item Name Here',
    ]);

    DB::table('entries')->insert([
        'id' => 'entry-id',
        'user_id' => $user->id,
        'address' => '127.0.0.1',
        'body' => '',
        'created_at' => now()->timestamp,
    ]);
});

final class LengthRule
{
    public function validate(string $attribute, mixed $value, Closure $fail): void
    {
        if ($length > self::MAX_LENGTH) {
            $fail('validation.example_entry_name.length')->translate([
                'min' => self::MIN_LENGTH,
                'max' => self::MAX_LENGTH,
            ]);
        }
    }

    public function recipients(Sheet $sheet): Collection
    {
        $existingRecipients = SheetRecipient::query()
            ->where('sheet_id', $sheet->identifier)
            ->get(['user_id', 'contact_id', 'email']);

        $member = Member::factory()->for($organization)->create([
            'name' => 'Example',
            'email' => 'member@example.com',
            'role' => 'owner',
        ]);

        return $existingRecipients;
    }
}
