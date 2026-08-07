<script lang="ts">
	import {
		Alert,
		Badge,
		Button,
		Card,
		ConfirmModal,
		Drawer,
		Field,
		Input,
		SecretField,
		SettingsSection,
		Table,
		icons,
		toast
	} from '@facile/muse';
	import { backend, type TokenInfo } from '$lib/backend';
	import { AGENT_PROMPT } from '$lib/agentPrompt';

	let tokens: TokenInfo[] = $state([]);
	const apiTokens = $derived(tokens.filter((t) => t.name !== 'session'));

	let createOpen = $state(false);
	let creating = $state(false);
	let newName = $state('');
	let createdToken = $state('');

	let revokeTarget = $state<TokenInfo | null>(null);
	let revokeOpen = $state(false);

	const endpoint = $derived(
		typeof window === 'undefined' ? '/api' : `${window.location.origin}/api`
	);

	$effect(() => {
		refresh();
	});

	async function refresh() {
		try {
			tokens = await backend.tokensList();
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not load tokens.');
		}
	}

	function openCreate() {
		/* Reset first: reopening the drawer must never re-show a token from a previous run. */
		createdToken = '';
		newName = '';
		createOpen = true;
	}

	async function create(event: Event) {
		event.preventDefault();
		if (!newName.trim()) return;
		creating = true;
		try {
			const result = await backend.tokensCreate(newName.trim());
			createdToken = result.token ?? '';
			newName = '';
			await refresh();
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not create the token.', {
				title: 'Token not created'
			});
		} finally {
			creating = false;
		}
	}

	async function revoke() {
		const target = revokeTarget;
		if (!target) return;
		try {
			await backend.tokensDelete(target.name);
			toast.success(`Token “${target.name}” revoked.`);
			revokeTarget = null;
			await refresh();
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not revoke that token.');
			throw e;
		}
	}

	async function copyPrompt() {
		try {
			await navigator.clipboard.writeText(AGENT_PROMPT);
			toast.success('Agent prompt copied.');
		} catch {
			toast.danger('The browser refused clipboard access. Select the text and copy it manually.');
		}
	}
</script>

<div class="flex flex-col gap-10">
	<SettingsSection
		title="Endpoint"
		description="Point the CLI or any HTTP client here. Not a secret — the token is."
	>
		<SecretField value={endpoint} sensitive={false} label="Base URL" />
	</SettingsSection>

	<SettingsSection
		title="Connect your agents"
		description="Paste this into an agent's master prompt (CLAUDE.md, AGENTS.md, GEMINI.md…) so it knows how to read, write and sync the shared brain."
		bare
	>
		{#snippet actions()}
			<Button variant="outline" icon={icons.copy} onclick={copyPrompt}>Copy prompt</Button>
		{/snippet}

		<Card class="overflow-hidden">
			<pre
				class="max-h-96 overflow-auto whitespace-pre-wrap font-fc-mono text-fc-xs leading-relaxed text-fc-fg">{AGENT_PROMPT}</pre>
		</Card>
		<p class="text-fc-xs text-fc-fg-muted">
			It assumes the <code class="font-fc-mono">mycelium</code> CLI is installed and logged in with
			one of the tokens below.
		</p>
	</SettingsSection>

	<SettingsSection
		title="API tokens"
		description="One token per machine. Revoking one never touches the others."
		bare
	>
		{#snippet actions()}
			<Button icon={icons.plus} onclick={openCreate}>New token</Button>
		{/snippet}

		{#if apiTokens.length === 0}
			<Alert tone="info">
				No tokens yet. The CLI needs one before a machine can sync with this brain.
			</Alert>
		{:else}
			<Table>
				<thead>
					<tr>
						<th scope="col">Name</th>
						<th scope="col">Scope</th>
						<th scope="col">Created</th>
						<th scope="col">Last seen</th>
						<th scope="col" class="text-right" aria-label="Actions"></th>
					</tr>
				</thead>
				<tbody>
					{#each apiTokens as token (token.name)}
						<tr>
							<td class="font-fc-mono font-medium text-fc-fg">{token.name}</td>
							<td>
								<Badge tone={token.scope === 'admin' ? 'accent' : 'neutral'}>
									{token.scope || 'machine'}
								</Badge>
							</td>
							<td class="whitespace-nowrap text-fc-fg-muted">
								{token.created_at?.slice(0, 10) || '—'}
							</td>
							<td class="whitespace-nowrap text-fc-fg-muted">
								{token.last_seen?.slice(0, 10) || 'never'}
							</td>
							<td class="text-right">
								<Button
									variant="ghost-danger"
									size="sm"
									icon={icons.revoke}
									aria-label="Revoke {token.name}"
									onclick={() => {
										revokeTarget = token;
										revokeOpen = true;
									}}
								>
									Revoke
								</Button>
							</td>
						</tr>
					{/each}
				</tbody>
			</Table>
		{/if}
	</SettingsSection>
</div>

<Drawer bind:open={createOpen} title="New API token" showClose>
	{#if createdToken}
		<div class="flex flex-col gap-4">
			<Alert tone="warning" title="Copy it now">
				This is the only time the token is shown. Mycelium stores a hash, so it cannot be shown
				again — losing it means issuing a new one.
			</Alert>

			<!-- The one-time token starts revealed and stays that way: hiding a value the user has
			     not copied yet is theatre. -->
			<SecretField
				value={createdToken}
				visible
				autoHideMs={0}
				label="Token"
				helper="Run `mycelium login` on the machine and paste it when asked."
			/>

			<div class="flex justify-end">
				<Button onclick={() => (createOpen = false)}>Done</Button>
			</div>
		</div>
	{:else}
		<form class="flex flex-col gap-4" onsubmit={create}>
			<Field label="Name" helper="Where the token will live — a machine, a pipeline, a script.">
				<Input bind:value={newName} placeholder="lucy" required disabled={creating} />
			</Field>
			<div class="flex justify-end gap-2 pt-1">
				<Button
					type="button"
					variant="ghost"
					disabled={creating}
					onclick={() => (createOpen = false)}
				>
					Cancel
				</Button>
				<Button type="submit" icon={icons.key} disabled={creating || newName.trim().length === 0}>
					{creating ? 'Creating…' : 'Create token'}
				</Button>
			</div>
		</form>
	{/if}
</Drawer>

<ConfirmModal
	bind:open={revokeOpen}
	tone="danger"
	title="Revoke “{revokeTarget?.name ?? ''}”?"
	description="That machine stops syncing immediately and any script using the token starts failing. It cannot be un-revoked — issue a new token and log in again."
	confirmLabel="Revoke token"
	cancelLabel="Keep it"
	onConfirm={revoke}
	onCancel={() => (revokeTarget = null)}
/>
