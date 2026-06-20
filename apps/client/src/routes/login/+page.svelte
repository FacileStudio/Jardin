<script lang="ts">
	import { backend } from '$lib/backend';
	import { goto } from '$app/navigation';

	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleLogin(e: Event) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			const { token } = await backend.login(password);
			localStorage.setItem('ruche.token', token);
			goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Login failed';
		} finally {
			loading = false;
		}
	}
</script>

<div class="flex min-h-screen items-center justify-center bg-bg">
	<form onsubmit={handleLogin} class="w-full max-w-sm space-y-4 rounded-xl border border-border bg-surface p-8">
		<h1 class="text-xl font-semibold text-fg">Ruche</h1>
		<p class="text-sm text-muted">Enter your admin password to continue.</p>

		{#if error}
			<p class="text-sm text-danger">{error}</p>
		{/if}

		<input
			type="password"
			bind:value={password}
			placeholder="Password"
			class="w-full rounded-lg border border-border bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
			autofocus
		/>

		<button
			type="submit"
			disabled={loading || !password}
			class="w-full rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-fg disabled:opacity-50"
		>
			{loading ? 'Signing in...' : 'Sign in'}
		</button>
	</form>
</div>
