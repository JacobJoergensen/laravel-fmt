<div>
    <div>
        <div>
            <div>
                <div>
                    <div>
                        <div>
                            <div>
                                @foreach ($rows as $row)
                                    @php
                                        $alphaCategory = AlphaBetaCategory::tryFrom($row->getCustomProperty('category', ''));
                                    @endphp
                                    @php($alphaCategory = AlphaBetaCategory::tryFrom($row->getCustomProperty('category', '')))
                                    @php($alphaCategory = AlphaBetaCategory::tryFrom($this->getCustomProperty('category', '')))
                                    @php($isFirst = $filter === '' && $index === 0)
                                @endforeach
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
