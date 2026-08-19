<script lang="ts">
	import { goto } from '$app/navigation';
	import { Alert, Button, EmptyState, Field, Input, Modal, icons, toast } from '@facile/muse';
	import { backend } from '$lib/backend';
	import EntityCard from '$lib/components/EntityCard.svelte';

	let skills: string[] = $state([]);
	let createOpen = $state(false);
	let draftName = $state('');
	let creating = $state(false);
	/* Separate from loadError: this one belongs to the create form, and showing a failed
	   list fetch inside a modal nobody opened is how a 403 came to look like an empty tree. */
	let error = $state('');
	let loadError = $state('');

	$effect(() => {
		backend
			.skillsList()
			.then((s) => (skills = s))
			.catch((e) => (loadError = e instanceof Error ? e.message : 'Could not load skills'));
	});

	function openCreate() {
		draftName = '';
		error = '';
		createOpen = true;
	}

	async function createSkill(event: Event) {
		event.preventDefault();
		const name = draftName.trim();
		if (!name) return;
		creating = true;
		error = '';
		try {
			const template = `---\nname: ${name}\ndescription: ""\ntriggers: ["/${name}"]\n---\n\n# ${name}\n`;
			await backend.skillSave(name, template);
			createOpen = false;
			toast.success(`Skill “${name}” created.`);
			goto(`/skills/${name}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create the skill';
		} finally {
			creating = false;
		}
	}
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-wrap items-start justify-between gap-4">
		<p class="min-w-0 text-fc-sm text-fc-fg-muted">
			Agent-agnostic capabilities, installed into each agent's own skill format.
		</p>
		<Button icon={icons.plus} onclick={openCreate}>New skill</Button>
	</div>

	{#if loadError}
		<Alert tone="danger" title="Could not load skills">{loadError}</Alert>
	{:else if skills.length === 0}
		<EmptyState
			icon={icons.bolt}
			title="No skills yet"
			description="Add one to teach every agent the same trick."
		>
			<Button variant="outline" icon={icons.plus} onclick={openCreate}>New skill</Button>
		</EmptyState>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 [&>*]:min-w-0">
			{#each skills as skill (skill)}
				<EntityCard
					href="/skills/{skill}"
					icon={icons.bolt}
					title={skill}
					meta="skills/{skill}.md"
				/>
			{/each}
		</div>
	{/if}
</div>

<Modal bind:open={createOpen} title="New skill" showClose>
	<form class="flex flex-col gap-4" onsubmit={createSkill}>
		<Field
			label="Name"
			helper="Becomes the slash command agents invoke it with."
			error={error || undefined}
		>
			<Input bind:value={draftName} placeholder="changelog" disabled={creating} required />
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
				{creating ? 'Creating…' : 'Create skill'}
			</Button>
		</div>
	</form>
</Modal>
