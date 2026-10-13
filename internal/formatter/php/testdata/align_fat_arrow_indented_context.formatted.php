<?php

class IndentedContext
{
    public function run($column, $rows, $v, $cond)
    {
        $chained = (new Collection($rows))
            ->map(fn ($column) => new Definition([
                'name'                 => $column['name'],
                'full_type_definition' => $column['type'],
                'nullable'             => $column['nullable'],
                'default'              => is_null($v)
                    ? null
                    : 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
                'autoIncrement' => $column['auto_increment'],
                'collation'     => $column['collation'],
            ]))
            ->all();

        $inArgs = doSomethingWithSeveral(
            $firstArgument,
            $secondArgument,
            new Definition([
                'name'                 => $column['name'],
                'full_type_definition' => $column['type'],
                'nullable'             => $column['nullable'],
                'default'              => is_null($v)
                    ? null
                    : 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
                'autoIncrement' => $column['auto_increment'],
                'collation'     => $column['collation'],
            ]),
            $lastArgument,
        );

        $inTernary = $cond
            ? new Definition([
                'name'                 => $column['name'],
                'full_type_definition' => $column['type'],
                'nullable'             => $column['nullable'],
                'default'              => is_null($v)
                    ? null
                    : 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
                'autoIncrement' => $column['auto_increment'],
                'collation'     => $column['collation'],
            ])
            : null;
    }
}
