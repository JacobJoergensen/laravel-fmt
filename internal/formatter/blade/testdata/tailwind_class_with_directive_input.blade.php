<div
    class="block @if ($tab !== 'trash') focus:outline-none focus-visible:ring-2 focus-visible:ring-[#cf7353] rounded @endif"
>
    Hi
</div>
<div class="form-input mt-2 @error('email') ring-1 border-red-500 @enderror block">Hi</div>
<div class="p-4 p-2 @if ($x) mt-2 m-1 @else mb-1 m-3 @endif">Hi</div>
<div class="text-white bg-blue-500 {{ $extra }} rounded @if ($y) p-4 m-2 @endif">Hi</div>
