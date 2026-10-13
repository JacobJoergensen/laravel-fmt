<div class="block @if ($tab !== 'trash') rounded focus:outline-none focus-visible:ring-2 focus-visible:ring-[#cf7353] @endif">
    Hi
</div>
<div class="form-input mt-2 @error('email') border-red-500 ring-1 @enderror block">Hi</div>
<div class="p-2 p-4 @if ($x) m-1 mt-2 @else m-3 mb-1 @endif">Hi</div>
<div class="bg-blue-500 text-white {{ $extra }} rounded @if ($y) m-2 p-4 @endif">Hi</div>
