<script lang="ts">
	import { backend } from '$lib/backend';
	import { getContext } from 'svelte';
	import type { RucheStatus } from '$lib/backend';

	let query = $state('');
	let results: { path: string; line: number; content: string }[] = $state([]);
	let index = $state('');
	let searching = $state(false);

	const getStatus = getContext<() => RucheStatus | null>('status');

	$effect(() => {
		backend.brainIndex().then((i) => (index = i)).catch(() => {});
	});

	async function search() {
		if (!query.trim()) return;
		searching = true;
		try {
			results = await backend.brainSearch(query);
		} catch {
			results = [];
		} finally {
			searching = false;
		}
	}
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold">Brain</h2>
		<p class="text-sm text-muted">Search and browse your agent memory.</p>
	</div>

	<form onsubmit={(e) => { e.preventDefault(); search(); }} class="flex gap-2">
		<input
			type="text"
			bind:value={query}
			placeholder="Search brain..."
			class="flex-1 rounded-lg border border-border bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
		/>
		<button
			type="submit"
			disabled={searching}
			class="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-fg disabled:opacity-50"
		>
			{searching ? '...' : 'Search'}
		</button>
	</form>

	{#if results.length > 0}
		<div class="space-y-1">
			{#each results as r}
				<div class="rounded-lg border border-border bg-surface px-3 py-2">
					<span class="text-xs font-medium text-primary">{r.path}:{r.line}</span>
					<p class="text-sm">{r.content}</p>
				</div>
			{/each}
		</div>
	{/if}

	{#if index}
		<div class="prose prose-sm max-w-none">
			<pre class="whitespace-pre-wrap rounded-lg bg-surface p-4 text-sm">{index}</pre>
		</div>
	{/if}
</div>
