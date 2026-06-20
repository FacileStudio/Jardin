<script lang="ts">
	import { backend, type TokenInfo } from '$lib/backend';

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
		<p class="text-sm text-muted-foreground">Manage API tokens for sync.</p>
	</div>

	{#if createdToken}
		<div class="rounded-lg border border-green-200 bg-green-50 p-4">
			<p class="mb-1 text-sm font-medium text-green-800">Token created — copy it now, it won't be shown again:</p>
			<div class="flex items-center gap-2">
				<code class="flex-1 rounded bg-background px-2 py-1 text-xs">{createdToken}</code>
				<button
					onclick={() => navigator.clipboard.writeText(createdToken)}
					class="rounded border border-border px-2 py-1 text-xs hover:bg-accent"
				>
					Copy
				</button>
			</div>
			<p class="mt-3 text-xs text-muted-foreground">
				To sync from another machine, run:
			</p>
			<pre class="mt-1 rounded bg-background p-2 font-mono text-xs">ruche login https://ruche.facile.studio</pre>
		</div>
	{/if}

	<section class="space-y-4">
		<h3 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">API Tokens</h3>

		<form onsubmit={(e) => { e.preventDefault(); createToken(); }} class="flex gap-2">
			<input
				type="text"
				bind:value={newTokenName}
				placeholder="Token name (e.g. lucy, ruche)"
				class="flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
			/>
			<button type="submit" class="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">
				Generate
			</button>
		</form>

		{#if tokens.length > 0}
			<div class="space-y-2">
				{#each tokens as token}
					<div class="flex items-center justify-between rounded-lg border border-border px-4 py-3">
						<div>
							<p class="text-sm font-medium">{token.name}</p>
							<p class="text-xs text-muted-foreground">Created {token.created_at}</p>
						</div>
						<button
							onclick={() => deleteToken(token.name)}
							class="text-xs text-destructive hover:underline"
						>
							Revoke
						</button>
					</div>
				{/each}
			</div>
		{:else}
			<p class="text-sm text-muted-foreground">No tokens yet.</p>
		{/if}
	</section>
</div>
