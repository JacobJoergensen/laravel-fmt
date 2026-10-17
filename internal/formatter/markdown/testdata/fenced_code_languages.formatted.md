# Snippets

```php
$total = sum([1, 2]);
```

```php
<?php

namespace App;

class Widget {}
```

```php
use App\One;
use App\Two;
```

```php
use App\Models\Widget;

#[Cached]
public function widget(): Widget
{
    return new Widget;
}
```

```php
Panel::make('details')->items([Field::make('name'), Field::make('email')])
```

```php
Field::make('name')   // Kept as written.
```

```PHP title="example.php"
if ($ready) {
    start();
}
```

~~~blade
<div class="box">{{ $name }}</div>
~~~

```html
<p>One</p>
<p>Two</p>
```

```css
.box {
    color: red;
}
```

```json
{ "name": "widget", "tags": [] }
```

```jsonc
{
  // The name.
  "name": "widget"
}
```

```yaml
items:
  - one
```

```xml
<root>
    <item/>
</root>
```

```shell
php   artisan   serve
```

```
$untagged=1;
```

```php
'key' => 'value',
```

<!-- laravel-fmt-ignore -->

```php
$kept   =   1;
```

- Step one:

  ```php
  $count = 3;
  ```

> ```json
> { "quoted": true }
> ```
