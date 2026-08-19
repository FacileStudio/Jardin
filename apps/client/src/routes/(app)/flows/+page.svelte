<script lang="ts">
	import { EmptyState, icons, toast } from '@facile/muse';
	import { backend } from '$lib/backend';
	import EntityCard from '$lib/components/EntityCard.svelte';

	let flows: string[] = $state([]);

	$effect(() => {
		backend
			.flowsList()
			.then((f) => (flows = f))
			.catch((e) => toast.danger(e instanceof Error ? e.message : 'Could not load flows.'));
	});
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-col gap-2">
		<h1 class="text-fc-2xl font-semibold text-fc-fg">Flows</h1>
		<p class="text-fc-sm text-fc-fg-muted">
			Recorded shell procedures, synced across every machine. Trust and run history stay on the
			machine that runs them.
		</p>
	</div>

	{#if flows.length === 0}
		<EmptyState
			icon={icons.plug}
			title="No flows yet"
			description="Create one with `mycelium flow add <name>` — it appears here once it syncs."
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 [&>*]:min-w-0">
			{#each flows as name (name)}
				<EntityCard href="/flows/{name}" icon={icons.plug} title={name} meta="flows/{name}.yml" />
			{/each}
		</div>
	{/if}
</div>
