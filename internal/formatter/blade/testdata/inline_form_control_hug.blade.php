<div>
    <a href="{{ route('widgets.show', $widget) }}" data-role="primary" data-target="widget-detail-panel" class="font-medium underline decoration-2">{{ $widget->title }}</a>
    <button type="button" data-role="secondary" data-target="widget-detail-panel" class="font-medium underline decoration-2">{{ $widget->label }}</button>
    <label for="widget-name-field" data-role="field-label" data-target="widget-detail-panel" class="font-medium underline decoration-2">{{ $widget->caption }}</label>
    <output name="widget-total" for="qty price" data-role="result" data-target="widget-detail-panel" class="font-medium underline">{{ $widget->total }}</output>
    <del data-role="removed" data-reason="superseded" data-target="widget-detail-panel" class="font-medium underline decoration-2">{{ $widget->oldName }}</del>
    <div data-role="wrapper" data-owner="widget-registry" data-target="widget-detail-panel" class="font-medium underline decoration-2">{{ $widget->body }}</div>
</div>
