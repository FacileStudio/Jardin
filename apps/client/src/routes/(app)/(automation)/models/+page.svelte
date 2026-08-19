<script lang="ts">
	import { Alert, EmptyState, icons } from '@facile/muse';
	import { backend } from '$lib/backend';
	import EntityCard from '$lib/components/EntityCard.svelte';

	let models: { type: string; path: string }[] = $state([]);
	let error = $state('');

	$effect(() => {
		backend
			.modelsList()
			.then((m) => (models = m))
			.catch((e) => (error = e instanceof Error ? e.message : 'Could not load models.'));
	});
</script>

<div class="flex flex-col gap-10">
	<p class="text-fc-sm text-fc-fg-muted">
		Typed step extensions a flow can run with <code class="font-fc-mono">type:</code>. Mycelium
		never executes these here — there is no bun on the server — so this is the source only.
	</p>

	{#if error}
		<Alert tone="danger" title="Could not load models">{error}</Alert>
	{:else if models.length === 0}
		<EmptyState
			icon={icons.code}
			title="No models yet"
			description="Add a TypeScript file under extensions/models — it appears here once it syncs."
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 [&>*]:min-w-0">
			{#each models as model (model.path)}
				<EntityCard
					href="/models/{model.path}"
					icon={icons.code}
					title={model.type}
					meta="extensions/models/{model.path}"
				/>
			{/each}
		</div>
	{/if}
</div>
