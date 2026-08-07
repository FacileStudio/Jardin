<script lang="ts">
	import { goto } from '$app/navigation';
	import { Badge, Button, Field, Input, Modal, Textarea, icons, toast } from '@facile/muse';
	import { backend, type Space, type SpaceRole } from '$lib/backend';
	import { setSpaces } from '$lib/space.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import EntityCard from '$lib/components/EntityCard.svelte';

	const roleTone = { owner: 'owner', admin: 'admin', member: 'neutral' } as const;

	let spaces: Space[] = $state([]);
	let createOpen = $state(false);
	let creating = $state(false);
	let draftName = $state('');
	let draftDescription = $state('');
	let createError = $state('');

	$effect(() => {
		backend
			.spacesList()
			.then((s) => {
				spaces = s;
				setSpaces(s);
			})
			.catch((e) => toast.danger(e instanceof Error ? e.message : 'Could not load spaces.'));
	});

	function openCreate() {
		draftName = '';
		draftDescription = '';
		createError = '';
		createOpen = true;
	}

	async function createSpace(event: Event) {
		event.preventDefault();
		const name = draftName.trim();
		if (!name) return;
		creating = true;
		createError = '';
		try {
			const space = await backend.spaceCreate(name, draftDescription.trim());
			createOpen = false;
			toast.success(`Space “${space.name}” created.`);
			goto(`/spaces/${space.id}`);
		} catch (e) {
			createError = e instanceof Error ? e.message : 'Could not create the space';
		} finally {
			creating = false;
		}
	}

	function tone(role: SpaceRole) {
		return roleTone[role] ?? 'neutral';
	}
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-wrap items-start justify-between gap-4">
		<div class="flex min-w-0 flex-col gap-2">
			<h1 class="text-fc-2xl font-semibold text-fc-fg">Spaces</h1>
			<p class="text-fc-sm text-fc-fg-muted">
				Shared memory trees. Invite teammates and scope memory, rules and skills per space.
			</p>
		</div>
		<Button icon={icons.plus} onclick={openCreate}>New space</Button>
	</div>

	{#if spaces.length === 0}
		<EmptyState
			icon={icons.usersGroup}
			title="No spaces yet"
			description="Create one to share a memory tree with the people you work with."
		>
			<Button variant="outline" icon={icons.plus} onclick={openCreate}>New space</Button>
		</EmptyState>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each spaces as space (space.id)}
				<EntityCard
					href="/spaces/{space.id}"
					icon={icons.usersGroup}
					title={space.name}
					meta={space.description || 'No description'}
					mono={false}
				>
					{#snippet trailing()}
						<Badge tone={tone(space.role)}>{space.role}</Badge>
					{/snippet}
				</EntityCard>
			{/each}
		</div>
	{/if}
</div>

<Modal bind:open={createOpen} title="New space" showClose>
	<form class="flex flex-col gap-4" onsubmit={createSpace}>
		<Field label="Name" error={createError || undefined}>
			<Input bind:value={draftName} placeholder="Facile" disabled={creating} required />
		</Field>
		<Field label="Description" helper="Optional — what this memory tree is for.">
			<Textarea
				bind:value={draftDescription}
				rows={3}
				placeholder="Team conventions, stack and shared projects."
				disabled={creating}
			/>
		</Field>
		<div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
			<Button
				type="button"
				variant="ghost"
				class="w-full sm:w-auto"
				onclick={() => (createOpen = false)}
			>
				Cancel
			</Button>
			<Button
				type="submit"
				icon={icons.plus}
				class="w-full sm:w-auto"
				disabled={creating || draftName.trim().length === 0}
			>
				{creating ? 'Creating…' : 'Create space'}
			</Button>
		</div>
	</form>
</Modal>
