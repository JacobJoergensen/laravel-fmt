<?php

final class Heading
{
    public function build(string $number, string $separator): string
    {
        $pattern = '/^(?:PAGE\s+)?('.$number.')(?:'.$separator.'('.$number
            .'))?\s*[-.:]*\s*[\(\[]?\s*DROP(?:PED)?\s*[\)\]]?\.?(?:\s+'.$number.')?\s*$/ui';

        $header = '<div style="width:100%;box-sizing:border-box;padding:0 0 0 0.35in;font-family:Arial,Helvetica,sans-serif;">'
            .'<img src="'.$this->theme->logoUrl().'" style="height:15pt;display:block;">'
            .'</div>';

        $footer = '<div style="width:100%;box-sizing:border-box;padding:0 0.5in;font-family:Arial,Helvetica,sans-serif;">'
            .'<span style="float:left;">'
            .e(__('report.footer', ['company_name' => $this->theme->name, 'title' => __('report.title')])).'</span>'
            .'</div>';

        $trailing = 'first piece of the message '
            .'second piece';

        $short = 'a'.$b.'c';

        if ($this->subscription) {
            $listeners['echo-private:subscription.'.$this->subscription.',SubscriptionUpdated'] = 'onSubscriptionUpdated';
        }

        $label = 'enums.shipment_statuses.long_prefix_segment.another_segment.yet_another_segment.final_segment.x'
            .match ($this->status) {
                'held' => '.held',
                default => '',
            };

        $title = Attribute::get(fn (): string => __('enums.shipment_statuses.'.match ($this->status) {
            'held', 'confirmed', 'packed', 'delivered', 'shipped', 'returned', 'cancelled' => $this->status,
            default => 'pending',
        }));

        return 'enums.shipment_statuses.long_prefix_segment.another_segment.'.match ($this->status) {
            'held', 'packed' => $this->status,
            default => 'pending',
        };
    }
}
