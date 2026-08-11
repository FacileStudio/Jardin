<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { backend, TOKEN_KEY } from '$lib/backend';

	const inputClass =
		'flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50';
	const labelClass = 'text-sm font-medium leading-none';
	const primaryButtonClass =
		'inline-flex h-10 w-full items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50';
	const outlineButtonClass =
		'inline-flex h-10 w-full items-center justify-center rounded-md border border-border bg-background px-4 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50';

	let password = $state('');
	let error = $state('');
	let busy = $state(false);
	let ssoOnly = $state(false);
	let oidcEnabled = $state(false);
	let configLoaded = $state(false);

	let redirect = $state('/memory');

	onMount(async () => {
		const target = new URL(window.location.href).searchParams.get('redirect');
		if (target && target.startsWith('/')) redirect = target;
		if (localStorage.getItem(TOKEN_KEY)) {
			try {
				await backend.authMe();
				goto(redirect);
				return;
			} catch {
				localStorage.removeItem(TOKEN_KEY);
			}
		}
		try {
			const res = await fetch('/api/auth/config');
			if (res.ok) {
				const cfg = await res.json();
				ssoOnly = (cfg.sso_only ?? false) || cfg.password_auth === false;
				oidcEnabled = cfg.oidc_enabled ?? false;
			}
		} catch {}
		configLoaded = true;
	});

	async function submit(e: Event) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			const { token } = await backend.login(password);
			localStorage.setItem(TOKEN_KEY, token);
			goto(redirect);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Something went wrong';
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head>
	<title>Connexion — Jardin</title>
</svelte:head>

<div class="flex min-h-screen">
	<div class="hidden lg:flex lg:w-1/2 flex-col bg-black px-12 py-10">
		<a href="/" class="flex items-center gap-3 mb-auto">
			<iconify-icon icon="solar:widget-6-bold-duotone" width="28" height="28" class="block text-white"
			></iconify-icon>
			<span class="text-xl font-bold font-heading tracking-tight text-white">Jardin</span>
		</a>

		<div class="mb-auto">
			<h2 class="text-4xl font-bold font-heading text-white leading-tight tracking-tight">
				Un cerveau.<br />Tous vos agents.
			</h2>
			<p class="mt-4 text-sm text-white/50 max-w-xs leading-relaxed">
				Mémoire, règles et compétences partagées entre vos agents IA.
			</p>
		</div>

		<p class="text-xs text-white/30">
			© {new Date().getFullYear()} Jardin par Facile.
		</p>
	</div>

	<div class="flex w-full lg:w-1/2 flex-col items-center justify-center px-8 py-12 bg-background">
		<div class="w-full max-w-sm">
			<div class="mb-8">
				<h1 class="text-2xl font-bold font-heading tracking-tight text-foreground">
					Bon retour
				</h1>
				<p class="mt-1.5 text-sm text-muted-foreground">
					{ssoOnly
						? 'Connectez-vous avec votre compte organisation.'
						: 'Connectez-vous à votre compte Jardin.'}
				</p>
			</div>

			{#if !configLoaded}
				<div class="h-40"></div>
			{:else}
				{#if !ssoOnly}
					<form onsubmit={submit} class="space-y-4">
						<div class="space-y-1.5">
							<label for="password" class={labelClass}>Mot de passe</label>
							<input
								id="password"
								type="password"
								bind:value={password}
								placeholder="••••••••"
								autocomplete="current-password"
								required
								disabled={busy}
								class={inputClass}
							/>
						</div>

						{#if error}
							<p class="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
								{error}
							</p>
						{/if}

						<button type="submit" disabled={busy} class={primaryButtonClass}>
							{busy ? 'Connexion…' : 'Se connecter'}
						</button>
					</form>
				{/if}

				{#if oidcEnabled}
					{#if !ssoOnly}
						<div class="my-5 flex items-center gap-3">
							<div class="h-px flex-1 bg-border"></div>
							<span class="text-xs text-muted-foreground">ou</span>
							<div class="h-px flex-1 bg-border"></div>
						</div>
					{/if}

					<a href="/api/auth/oidc" class="block">
						<button type="button" class={outlineButtonClass}>Continuer avec SSO</button>
					</a>
				{/if}
			{/if}
		</div>
	</div>
</div>
