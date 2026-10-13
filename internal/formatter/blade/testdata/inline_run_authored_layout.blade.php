<form method="POST" action="{{ route('login') }}">
    <div>
        <x-input-label for="email" :value="__('Email')" />
    </div>
    <div class="flex items-center justify-end mt-4">
        <x-primary-button class="ms-3">
            {{ __('Log in') }}
        </x-primary-button>
    </div>
    <div class="flex gap-2">
        <button type="submit">Save</button>
        <button type="button">Cancel</button>
    </div>
    <p>Forgot it? <a href="{{ route('password.request') }}">Reset</a> or <button type="button">retry</button> now.</p>
</form>
