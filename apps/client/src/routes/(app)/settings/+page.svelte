<script lang="ts">
	import { backend, type TokenInfo, type RucheStatus } from '$lib/backend';
	import { getContext } from 'svelte';

	const getStatus = getContext<() => RucheStatus | null>('status');
	let status = $derived(getStatus());

	let tokens: TokenInfo[] = $state([]);
	let newTokenName = $state('');
	let createdToken = $state('');

	$effect(() => {
		backend.tokensList().then((t) => (tokens = t)).catch(() => {});
	});

	async function createToken() {
		if (!newTokenName.trim()) return;
		try {
			const result = await backend.tokensCreate(newTokenName);
			createdToken = result.token;
			newTokenName = '';
			tokens = await backend.tokensList();
		} catch {
			createdToken = '';
		}
	}

	async function deleteToken(name: string) {
		await backend.tokensDelete(name);
		tokens = await backend.tokensList();
	}
</script>

<div class="space-y-8">
	<div>
		<h2 class="text-xl font-semibold">Settings</h2>
		<p class="text-sm text-muted">Manage tokens and sync configuration.</p>
	</div>

	<section class="space-y-4">
		<h3 class="text-sm font-semibold uppercase tracking-wide text-muted">Sync Configuration</h3>
		<div class="rounded-lg border border-border bg-surface p-4 text-sm">
			<p><span class="font-medium">Machine:</span> {status?.machine || 'not set'}</p>
			<p><span class="font-medium">Sync URL:</span> {status?.url || 'not configured'}</p>
		</div>

		{#if createdToken}
			<div class="rounded-lg border border-success/30 bg-success/5 p-4">
				<p class="mb-1 text-sm font-medium text-success">Token created — copy it now, it won't be shown again:</p>
				<div class="flex items-center gap-2">
					<code class="flex-1 rounded bg-bg px-2 py-1 text-xs">{createdToken}</code>
					<button
						onclick={() => navigator.clipboard.writeText(createdToken)}
						class="rounded border border-border px-2 py-1 text-xs hover:bg-bg"
					>
						Copy
					</button>
				</div>
				<p class="mt-3 text-xs text-muted">
					To sync from another machine, add this to <code>~/.ruche/ruche.toml</code>:
				</p>
				<pre class="mt-1 rounded bg-bg p-2 text-xs">url = "{status?.url || 'https://ruche.yourdomain.com'}"
token = "{createdToken}"</pre>
			</div>
		{/if}
	</section>

	<section class="space-y-4">
		<h3 class="text-sm font-semibold uppercase tracking-wide text-muted">API Tokens</h3>

		<form onsubmit={(e) => { e.preventDefault(); createToken(); }} class="flex gap-2">
			<input
				type="text"
				bind:value={newTokenName}
				placeholder="Token name (e.g. lucy, ruche)"
				class="flex-1 rounded-lg border border-border bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
			/>
			<button type="submit" class="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-fg">
				Generate
			</button>
		</form>

		{#if tokens.length > 0}
			<div class="space-y-2">
				{#each tokens as token}
					<div class="flex items-center justify-between rounded-lg border border-border bg-surface px-4 py-3">
						<div>
							<p class="text-sm font-medium">{token.name}</p>
							<p class="text-xs text-muted">Created {token.created_at}</p>
						</div>
						<button
							onclick={() => deleteToken(token.name)}
							class="text-xs text-danger hover:underline"
						>
							Revoke
						</button>
					</div>
				{/each}
			</div>
		{:else}
			<p class="text-sm text-muted">No tokens yet.</p>
		{/if}
	</section>
</div>
