<script lang="ts">
	import { Button, Card, Input, icons, toast } from '@facile/muse';
	import { backend, type MemorySearchHit } from '$lib/backend';
	import IndexStatus from '$lib/components/IndexStatus.svelte';

	let query = $state('');
	let results: MemorySearchHit[] = $state([]);
	let searched = $state(false);
	let searchDegraded = $state(false);
	let searching = $state(false);

	async function search(event: Event) {
		event.preventDefault();
		if (!query.trim()) return;
		searching = true;
		try {
			const answer = await backend.memorySearch(query);
			results = answer.results;
			searchDegraded = answer.degraded;
		} catch (e) {
			results = [];
			toast.danger(e instanceof Error ? e.message : 'Search failed.');
		} finally {
			searching = false;
			searched = true;
		}
	}
</script>

<section class="flex flex-col gap-4">
	<IndexStatus />

	<form class="flex flex-col gap-3 sm:flex-row" onsubmit={search}>
		<div class="min-w-0 flex-1">
			<Input bind:value={query} placeholder="Search memory…" aria-label="Search memory" />
		</div>
		<Button
			type="submit"
			variant="outline"
			icon={icons.search}
			disabled={searching || query.trim().length === 0}
		>
			{searching ? 'Searching…' : 'Search'}
		</Button>
	</form>

	{#if searchDegraded && searched}
		<p class="text-fc-xs text-fc-fg-muted">
			Matched words only — the semantic half is unavailable.
		</p>
	{/if}

	{#if results.length > 0}
		<div class="flex flex-col gap-2">
			{#each results as result (result.path + ':' + result.line)}
				{@const rel = result.path.startsWith('memory/')
					? result.path.slice('memory/'.length)
					: result.path}
				<!-- A result row is a card that navigates, just a denser one than the file
				     grid: px-4/py-3 overrides Card's p-5 rather than restating the surface. -->
				<Card href="/memory/{rel}" class="flex flex-col gap-1 px-4 py-3">
					<span class="font-fc-mono text-fc-xs text-fc-fg-muted">
						{result.path}:{result.line}
					</span>
					{#if result.heading}
						<span class="text-fc-sm font-medium text-fc-fg">{result.heading}</span>
					{/if}
					<span class="truncate text-fc-sm text-fc-fg-muted">{result.excerpt}</span>
				</Card>
			{/each}
		</div>
	{:else if searched && !searching}
		<Card class="text-fc-sm text-fc-fg-muted">Nothing matched “{query}”.</Card>
	{/if}
</section>
