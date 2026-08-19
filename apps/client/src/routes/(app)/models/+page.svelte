<script lang="ts">
	import { EmptyState, icons, toast } from '@facile/muse';
	import { backend } from '$lib/backend';
	import EntityCard from '$lib/components/EntityCard.svelte';

	let models: { type: string; path: string }[] = $state([]);

	$effect(() => {
		backend
			.modelsList()
			.then((m) => (models = m))
			.catch((e) => toast.danger(e instanceof Error ? e.message : 'Could not load models.'));
	});
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-col gap-2">
		<h1 class="text-fc-2xl font-semibold text-fc-fg">Models</h1>
		<p class="text-fc-sm text-fc-fg-muted">
			Typed step extensions a flow can run with <code class="font-fc-mono">type:</code>. Jardin
			never executes these here — there is no bun on the server — so this is the source only.
		</p>
	</div>

	{#if models.length === 0}
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
