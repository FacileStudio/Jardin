<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { TOKEN_KEY } from '$lib/backend';

	onMount(() => {
		const params = new URLSearchParams(location.hash.replace(/^#/, ''));
		const token = params.get('token');
		if (!token) {
			goto('/login');
			return;
		}
		localStorage.setItem(TOKEN_KEY, token);
		history.replaceState(null, '', '/auth/callback');
		goto('/memory');
	});
</script>

<svelte:head>
	<title>Connexion — Jardin</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-background">
	<p class="text-sm text-muted-foreground">Connexion…</p>
</div>
