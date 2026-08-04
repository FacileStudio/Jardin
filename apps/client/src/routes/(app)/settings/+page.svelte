<script lang="ts">
	import { backend, type EmitterStatus, type NookSettings, type TokenInfo } from '$lib/backend';
	import { AGENT_PROMPT } from '$lib/agentPrompt';

	let tokens: TokenInfo[] = $state([]);
	let apiTokens = $derived(tokens.filter((t) => t.name !== 'session'));
	let newTokenName = $state('');
	let createdToken = $state('');
	let promptCopied = $state(false);

	let nookLoaded = $state(false);
	let nookDenied = $state(false);
	let nookEnabled = $state(false);
	let nookInstance = $state('');
	let nookSecret = $state('');
	let nookUserEmail = $state('');
	let nookEmitSince = $state('');
	let machineEmails: { machine: string; email: string }[] = $state([]);
	let nookStatus: EmitterStatus | null = $state(null);
	let nookError = $state('');
	let nookSaving = $state(false);

	async function copyPrompt() {
		await navigator.clipboard.writeText(AGENT_PROMPT);
		promptCopied = true;
		setTimeout(() => (promptCopied = false), 2000);
	}

	$effect(() => {
		backend.tokensList().then((t) => (tokens = t)).catch(() => {});
	});

	$effect(() => {
		backend
			.settingsGet()
			.then((s) => {
				applyNook(s.nook, s.status);
				nookLoaded = true;
			})
			.catch(() => (nookDenied = true));
	});

	function applyNook(nook: NookSettings, status: EmitterStatus) {
		nookEnabled = nook.enabled;
		nookInstance = nook.instance;
		nookSecret = nook.secret;
		nookUserEmail = nook.user_email;
		nookEmitSince = nook.emit_since ?? '';
		machineEmails = Object.entries(nook.machine_emails ?? {}).map(([machine, email]) => ({ machine, email }));
		nookStatus = status;
	}

	async function saveNook() {
		nookSaving = true;
		nookError = '';
		try {
			const emails: Record<string, string> = {};
			for (const row of machineEmails) {
				if (row.machine.trim()) emails[row.machine.trim()] = row.email.trim();
			}
			const s = await backend.settingsSave({
				enabled: nookEnabled,
				instance: nookInstance,
				secret: nookSecret,
				user_email: nookUserEmail,
				machine_emails: emails,
				emit_since: nookEmitSince || undefined
			});
			applyNook(s.nook, s.status);
		} catch (e) {
			nookError = e instanceof Error ? e.message : 'Save failed';
		} finally {
			nookSaving = false;
		}
	}

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
				<code class="rounded bg-accent px-1 py-0.5 text-xs">jardin</code> CLI is installed and
				logged in (<code class="rounded bg-accent px-1 py-0.5 text-xs">jardin login https://jardin.facile.studio</code>).
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
			<pre class="mt-1 rounded bg-background p-2 font-mono text-xs">jardin login https://jardin.facile.studio</pre>
		</div>
	{/if}

	<section class="space-y-4">
		<h3 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">API Tokens</h3>

		<form onsubmit={(e) => { e.preventDefault(); createToken(); }} class="flex gap-2">
			<input
				type="text"
				bind:value={newTokenName}
				placeholder="Token name (e.g. lucy, jardin)"
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

	<section class="space-y-4">
		<div>
			<h3 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Nook sync</h3>
			<p class="mt-1 text-sm text-muted-foreground">
				Publish sealed agent sessions to the Nook event bus so Sablier can turn them into time entries.
			</p>
		</div>

		{#if nookDenied}
			<p class="text-sm text-muted-foreground">Admin login required.</p>
		{:else if nookLoaded}
			{#if nookStatus}
				<div class="flex flex-wrap items-center gap-2 text-sm">
					<span class="size-2 rounded-full {nookStatus.connected ? 'bg-green-500' : 'bg-muted-foreground/40'}"></span>
					<span class={nookStatus.connected ? '' : 'text-muted-foreground'}>
						{nookStatus.connected ? 'Connected' : 'Disconnected'}
					</span>
					<span class="text-muted-foreground">{nookStatus.pending} sessions pending</span>
					{#if nookStatus.last_error}
						<span class="text-xs text-destructive">{nookStatus.last_error}</span>
					{/if}
				</div>
			{/if}

			<form onsubmit={(e) => { e.preventDefault(); saveNook(); }} class="space-y-4">
				<label class="flex items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={nookEnabled} class="size-4 rounded border-input accent-primary" />
					Enabled
				</label>

				<div class="grid gap-3 sm:grid-cols-2">
					<label class="space-y-1">
						<span class="text-xs font-medium text-muted-foreground">Instance URL</span>
						<input
							type="text"
							bind:value={nookInstance}
							placeholder="https://nook.facile.studio"
							class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
						/>
					</label>
					<label class="space-y-1">
						<span class="text-xs font-medium text-muted-foreground">Secret</span>
						<input
							type="password"
							bind:value={nookSecret}
							class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
						/>
					</label>
					<label class="space-y-1">
						<span class="text-xs font-medium text-muted-foreground">Attribution email</span>
						<input
							type="text"
							bind:value={nookUserEmail}
							placeholder="you@example.com"
							class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
						/>
					</label>
				</div>

				<div class="space-y-2">
					<p class="text-xs font-medium text-muted-foreground">Machine email overrides</p>
					{#each machineEmails as row, i}
						<div class="flex gap-2">
							<input
								type="text"
								bind:value={row.machine}
								placeholder="machine"
								class="w-40 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
							/>
							<input
								type="text"
								bind:value={row.email}
								placeholder="email"
								class="flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
							/>
							<button
								type="button"
								onclick={() => (machineEmails = machineEmails.filter((_, j) => j !== i))}
								class="rounded-lg border border-border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent"
							>
								Remove
							</button>
						</div>
					{/each}
					<button
						type="button"
						onclick={() => (machineEmails = [...machineEmails, { machine: '', email: '' }])}
						class="rounded-lg border border-border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent"
					>
						Add override
					</button>
				</div>

				{#if nookEmitSince}
					<p class="text-xs text-muted-foreground">Emitting sessions ended after {nookEmitSince}</p>
				{/if}

				<div class="flex items-center gap-3">
					<button
						type="submit"
						disabled={nookSaving}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-primary px-3.5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
					>
						{nookSaving ? 'Saving…' : 'Save'}
					</button>
					{#if nookError}
						<span class="text-sm text-destructive">{nookError}</span>
					{/if}
				</div>
			</form>
		{/if}
	</section>
</div>
