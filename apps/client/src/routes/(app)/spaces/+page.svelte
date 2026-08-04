<script lang="ts">
	import Icon from '@iconify/svelte';
	import { goto } from '$app/navigation';
	import { backend, type Space } from '$lib/backend';
	import { setSpaces } from '$lib/space.svelte';

	let spaces: Space[] = $state([]);
	let error = $state('');

	$effect(() => {
		backend
			.spacesList()
			.then((s) => {
				spaces = s;
				setSpaces(s);
			})
			.catch(() => {});
	});

	async function addSpace() {
		const name = prompt('Space name:');
		if (!name) return;
		const description = prompt('Description (optional):') ?? '';
		error = '';
		try {
			const space = await backend.spaceCreate(name, description);
			goto(`/spaces/${space.id}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to create space';
		}
	}
</script>

<div class="space-y-7">
	<div class="flex items-end justify-between gap-4">
		<div>
			<h2 class="text-2xl font-semibold tracking-tight">Spaces</h2>
			<p class="mt-1 text-sm text-muted-foreground">Shared memory trees — invite teammates and scope rules, skills, and memory per space.</p>
		</div>
		<button onclick={addSpace} class="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-primary px-3.5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90">
			<Icon icon="mdi:plus" class="size-4" />
			New space
		</button>
	</div>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	{#if spaces.length === 0}
		<div class="rounded-xl border border-dashed border-border p-12 text-center">
			<Icon icon="solar:users-group-rounded-linear" class="mx-auto size-6 text-muted-foreground/50" />
			<p class="mt-2 text-sm text-muted-foreground">No spaces yet. Create one to share a memory tree with others.</p>
		</div>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each spaces as space}
				<a
					href="/spaces/{space.id}"
					class="group rounded-xl border border-border bg-background p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-sm"
				>
					<div class="flex items-center justify-between">
						<div class="flex size-9 items-center justify-center rounded-lg bg-accent">
							<Icon icon="solar:users-group-rounded-linear" class="size-[18px] text-foreground" />
						</div>
						<Icon icon="solar:alt-arrow-right-linear" class="size-4 text-muted-foreground/30 transition-all group-hover:translate-x-0.5 group-hover:text-muted-foreground" />
					</div>
					<div class="mt-3 flex items-center gap-2">
						<p class="truncate font-medium">{space.name}</p>
						<span class="rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground">{space.role}</span>
					</div>
					<p class="truncate text-xs text-muted-foreground">{space.description || 'No description'}</p>
				</a>
			{/each}
		</div>
	{/if}
</div>
