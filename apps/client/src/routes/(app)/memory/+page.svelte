<script lang="ts">
	import Icon from '@iconify/svelte';
	import { backend } from '$lib/backend';

	let query = $state('');
	let results: { path: string; line: number; content: string }[] = $state([]);
	let index = $state('');
	let searching = $state(false);

	let indexBody = $derived(index.replace(/^#\s+.*(\r?\n)?/, '').trim());

	$effect(() => {
		backend.memoryIndex().then((i) => (index = i)).catch(() => {});
	});

	async function search() {
		if (!query.trim()) return;
		searching = true;
		try {
			results = await backend.memorySearch(query);
		} catch {
			results = [];
		} finally {
			searching = false;
		}
	}
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-xl font-semibold">Memory</h2>
		<p class="text-sm text-muted-foreground">Search and browse your agent memory.</p>
	</div>

	<form onsubmit={(e) => { e.preventDefault(); search(); }} class="flex gap-2">
		<input
			type="text"
			bind:value={query}
			placeholder="Search memory..."
			class="flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
		/>
		<button
			type="submit"
			disabled={searching}
			class="inline-flex items-center gap-1.5 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
		>
			<Icon icon="solar:magnifer-linear" class="size-4" />
			{searching ? '...' : 'Search'}
		</button>
	</form>

	{#if results.length > 0}
		<div class="space-y-1">
			{#each results as r}
				<div class="rounded-lg border border-border px-3 py-2">
					<span class="text-xs font-medium text-primary">{r.path}:{r.line}</span>
					<p class="text-sm">{r.content}</p>
				</div>
			{/each}
		</div>
	{/if}

	{#if indexBody}
		<div>
			<pre class="whitespace-pre-wrap rounded-lg border border-border bg-accent p-4 text-sm">{indexBody}</pre>
		</div>
	{:else if results.length === 0}
		<div class="rounded-lg border border-dashed border-border p-8 text-center">
			<p class="text-sm text-muted-foreground">No memories yet. Your agents will fill this in as they learn.</p>
		</div>
	{/if}
</div>
