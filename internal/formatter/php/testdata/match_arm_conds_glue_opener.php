<?php

class Stamper
{
    public function stamp(Field $field, Page $page, Signer $signer): void
    {
        match ($field->type) {
            FieldKind::Signature, FieldKind::Initials => $this->drawSignature($page, $signer, $field->x, $field->y, (float) $field->width, (float) $field->height),
            FieldKind::Checkbox, FieldKind::Radio => ['label' => __('fields.checkbox.label'), 'icon' => 'check-square', 'state' => $field->value],
            FieldKind::DateSignedWithTimezone, FieldKind::DateSignedWithoutTimezone, FieldKind::DateOnly => $this->drawDate($page, $signer->signed_at->toDateString()),
            FieldKind::Text,
            FieldKind::Note => $this->drawText($page, (string) $field->value),
            FieldKind::Stamp, FieldKind::Seal => null,
            default => $this->drawText($page, (string) ($field->value ?? ''), $field->x, $field->y, (float) $field->height),
        };
    }
}
