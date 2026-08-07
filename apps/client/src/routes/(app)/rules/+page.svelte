<script lang="ts">
	import { goto } from '$app/navigation';
	import { Button, Field, Input, Modal, icons, toast } from '@facile/muse';
	import { backend } from '$lib/backend';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import EntityCard from '$lib/components/EntityCard.svelte';

	let rules: string[] = $state([]);
	let createOpen = $state(false);
	let draftName = $state('');
	let creating = $state(false);
	let error = $state('');

	$effect(() => {
		backend
			.rulesList()
			.then((r) => (rules = r))
			.catch((e) => (error = e instanceof Error ? e.message : 'Could not load rules'));
	});

	function openCreate() {
		draftName = '';
		error = '';
		createOpen = true;
	}

	async function createRule(event: Event) {
		event.preventDefault();
		const name = draftName.trim();
		if (!name) return;
		creating = true;
		error = '';
		try {
			await backend.ruleSave(name, `# ${name}\n`);
			createOpen = false;
			toast.success(`Rule “${name}” created.`);
			goto(`/rules/${name}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create the rule';
		} finally {
			creating = false;
		}
	}
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-wrap items-start justify-between gap-4">
		<div class="flex min-w-0 flex-col gap-2">
			<h1 class="text-fc-2xl font-semibold text-fc-fg">Rules</h1>
			<p class="text-fc-sm text-fc-fg-muted">
				Modular instructions, concatenated into every agent config in filename order.
			</p>
		</div>
		<Button icon={icons.plus} onclick={openCreate}>New rule</Button>
	</div>

	{#if rules.length === 0}
		<EmptyState
			icon={icons.shield}
			title="No rules yet"
			description="Create one to shape how every agent behaves."
		>
			<Button variant="outline" icon={icons.plus} onclick={openCreate}>New rule</Button>
		</EmptyState>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each rules as rule (rule)}
				<EntityCard
					href="/rules/{rule}"
					icon={icons.shield}
					title={rule}
					meta="rules/{rule}.md"
				/>
			{/each}
		</div>
	{/if}
</div>

<Modal bind:open={createOpen} title="New rule" showClose>
	<form class="flex flex-col gap-4" onsubmit={createRule}>
		<Field
			label="Name"
			helper="Prefix it to control the order agents read it in — 30-testing, 40-git."
			error={error || undefined}
		>
			<Input bind:value={draftName} placeholder="30-testing" disabled={creating} required />
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
				{creating ? 'Creating…' : 'Create rule'}
			</Button>
		</div>
	</form>
</Modal>
