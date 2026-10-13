<div>
<p>Hi @js($x) there</p>
<li>Total: @money($x) incl.</li>
<p>@lang('messages.welcome',['name'=>$user->name])</p>
<span>Hi    @js(  [1,2] )    there</span>
<p>Price: @js($x)!</p>
<td>@lang("a") <a href="#">x</a></td>
<p>Read @lang("a") and <strong>b</strong> here.</p>
<p>@csrf
(note)</p>
<p><em>x</em> @csrf
(note)</p>
<p>Hi <b>there</b> @if($a) yes</p>
</div>
