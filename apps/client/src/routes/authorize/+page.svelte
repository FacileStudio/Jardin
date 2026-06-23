<script lang="ts">
	import Icon from '@iconify/svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { backend } from '$lib/backend';

	let code = $state('');
	let machine = $state('');
	let ip = $state('');
	let status = $state('');
	let loaded = $state(false);
	let busy = $state(false);
	let error = $state('');
	let done: '' | 'approved' | 'denied' = $state('');

	onMount(async () => {
		const token = localStorage.getItem('ruche.token');
		const url = new URL(window.location.href);
		code = url.searchParams.get('code') ?? '';

		if (!token) {
			goto('/login?redirect=' + encodeURIComponent('/authorize' + url.search));
			return;
		}
		if (code) await loadInfo();
		loaded = true;
	});

	async function loadInfo() {
		error = '';
		try {
			const info = await backend.deviceInfo(code);
			machine = info.machine;
			ip = info.ip;
			status = info.status;
		} catch {
			error = 'That code was not found or has expired.';
			machine = '';
			status = '';
		}
	}

	async function approve() {
		busy = true;
		error = '';
		try {
			await backend.deviceApprove(code);
			done = 'approved';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Something went wrong';
		} finally {
			busy = false;
		}
	}

	async function deny() {
		busy = true;
		error = '';
		try {
			await backend.deviceDeny(code);
			done = 'denied';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Something went wrong';
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head>
	<title>Authorize a machine — Ruche</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-background px-6 py-12">
	<div class="w-full max-w-sm">
		<a href="/memory" class="mb-8 flex items-center justify-center gap-2.5">
			<Icon icon="solar:graph-new-bold-duotone" class="size-7 text-foreground" />
			<span class="text-xl font-bold tracking-tight">Ruche</span>
		</a>

		{#if !loaded}
			<div class="h-40"></div>
		{:else if done === 'approved'}
			<div class="rounded-xl border border-border bg-card p-6 text-center">
				<Icon icon="solar:check-circle-bold" class="mx-auto size-10 text-green-600" />
				<h1 class="mt-3 text-lg font-semibold">Machine authorized</h1>
				<p class="mt-1 text-sm text-muted-foreground">
					<span class="font-medium text-foreground">{machine}</span> can now sync. Return to your
					terminal — it will finish automatically.
				</p>
			</div>
		{:else if done === 'denied'}
			<div class="rounded-xl border border-border bg-card p-6 text-center">
				<Icon icon="solar:close-circle-bold" class="mx-auto size-10 text-muted-foreground" />
				<h1 class="mt-3 text-lg font-semibold">Request denied</h1>
				<p class="mt-1 text-sm text-muted-foreground">No token was issued.</p>
			</div>
		{:else}
			<div class="mb-6 text-center">
				<h1 class="text-xl font-bold tracking-tight">Authorize a machine</h1>
				<p class="mt-1.5 text-sm text-muted-foreground">
					Confirm the code shown in your terminal to let this machine sync.
				</p>
			</div>

			{#if !machine}
				<form
					onsubmit={(e) => {
						e.preventDefault();
						loadInfo();
					}}
					class="space-y-3"
				>
					<input
						bind:value={code}
						placeholder="XXXX-XXXX"
						autocomplete="off"
						class="h-11 w-full rounded-md border border-input bg-background px-3 text-center font-mono text-lg uppercase tracking-widest outline-none focus-visible:ring-2 focus-visible:ring-ring"
					/>
					<button
						type="submit"
						class="inline-flex h-10 w-full items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
					>
						Continue
					</button>
				</form>
			{:else}
				<div class="rounded-xl border border-border bg-card p-5">
					<div class="flex items-center gap-3">
						<div class="flex size-10 items-center justify-center rounded-lg bg-accent">
							<Icon icon="solar:server-square-linear" class="size-5" />
						</div>
						<div>
							<p class="text-sm text-muted-foreground">Machine requesting access</p>
							<p class="font-semibold">{machine}</p>
						</div>
					</div>
					<div class="mt-3 space-y-1 font-mono text-sm text-muted-foreground">
						<p>code <span class="text-foreground">{code}</span></p>
						{#if ip}<p>from <span class="text-foreground">{ip}</span></p>{/if}
					</div>

					<p class="mt-4 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
						Only authorize if you just ran <code>ruche login</code> on this machine. Approving grants
						it ongoing sync access to your shared brain.
					</p>

					<div class="mt-5 flex gap-2">
						<button
							onclick={deny}
							disabled={busy}
							class="inline-flex h-10 flex-1 items-center justify-center rounded-md border border-border bg-background px-4 text-sm font-medium hover:bg-accent disabled:opacity-50"
						>
							Deny
						</button>
						<button
							onclick={approve}
							disabled={busy}
							class="inline-flex h-10 flex-1 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
						>
							{busy ? 'Authorizing…' : 'Authorize'}
						</button>
					</div>
				</div>
			{/if}

			{#if error}
				<p class="mt-4 text-center text-sm text-destructive">{error}</p>
			{/if}
		{/if}
	</div>
</div>
