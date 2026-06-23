<script lang="ts">
	import { backend, type TokenInfo } from '$lib/backend';
	import { AGENT_PROMPT } from '$lib/agentPrompt';

	let tokens: TokenInfo[] = $state([]);
	let apiTokens = $derived(tokens.filter((t) => t.name !== 'session'));
	let newTokenName = $state('');
	let createdToken = $state('');
	let promptCopied = $state(false);

	async function copyPrompt() {
		await navigator.clipboard.writeText(AGENT_PROMPT);
		promptCopied = true;
		setTimeout(() => (promptCopied = false), 2000);
	}

	$effect(() => {
		backend.tokensList().then((t) => (tokens = t)).catch(() => {});
	});

	async function createToken() {
		if (!newTokenName.trim()) return;
		try {
			const result = await backend.tokensCreate(newTokenName);
			createdToken = result.token ?? '';
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
		<h2 class="text-2xl font-semibold tracking-tight">Settings</h2>
		<p class="text-sm text-muted-foreground">Connect your agents and manage sync tokens.</p>
	</div>

	<section class="space-y-4">
		<div>
			<h3 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Connect your agents</h3>
			<p class="mt-1 text-sm text-muted-foreground">
				Paste this into an agent's master prompt (CLAUDE.md, AGENTS.md, GEMINI.md…) so it knows
				how to read, write, and sync the shared brain. It assumes the
				<code class="rounded bg-accent px-1 py-0.5 text-xs">ruche</code> CLI is installed and
				logged in (<code class="rounded bg-accent px-1 py-0.5 text-xs">ruche login https://ruche.facile.studio</code>).
			</p>
		</div>

		<div class="relative">
			<button
				onclick={copyPrompt}
				class="absolute right-2 top-2 z-10 rounded-md border border-border bg-background px-2.5 py-1 text-xs font-medium hover:bg-accent"
			>
				{promptCopied ? 'Copied!' : 'Copy'}
			</button>
			<pre class="max-h-96 overflow-auto whitespace-pre-wrap rounded-lg border border-border bg-accent p-4 pr-16 text-xs leading-relaxed">{AGENT_PROMPT}</pre>
		</div>
	</section>

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

		{#if apiTokens.length > 0}
			<div class="space-y-2">
				{#each apiTokens as token}
					<div class="flex items-center justify-between rounded-lg border border-border px-4 py-3">
						<div>
							<div class="flex items-center gap-2">
								<p class="text-sm font-medium">{token.name}</p>
								{#if token.scope}
									<span class="rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground">{token.scope}</span>
								{/if}
							</div>
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
