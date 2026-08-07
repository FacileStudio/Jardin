<script lang="ts">
	import { goto } from '$app/navigation';
	import { Button, Card, Field, Input, Modal, Select, icons, toast } from '@facile/muse';
	import { backend, type FileEntry } from '$lib/backend';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import EntityCard from '$lib/components/EntityCard.svelte';

	const FOLDERS = ['bugs', 'tools', 'projects', 'conventions', 'syntheses'];

	let query = $state('');
	let results: { path: string; line: number; content: string }[] = $state([]);
	let searched = $state(false);
	let searching = $state(false);
	let files: FileEntry[] = $state([]);

	let createOpen = $state(false);
	let creating = $state(false);
	let draftFolder = $state('');
	let draftName = $state('');
	let createError = $state('');

	$effect(() => {
		backend
			.syncTree()
			.then(
				(t) =>
					(files = t
						.filter((f) => f.path.startsWith('memory/'))
						.sort((a, b) => a.path.localeCompare(b.path)))
			)
			.catch((e) =>
				toast.danger(e instanceof Error ? e.message : 'Could not load the memory tree.')
			);
	});

	const grouped = $derived.by(() => {
		const groups: Record<string, FileEntry[]> = {};
		for (const f of files) {
			const parts = f.path.split('/');
			const folder = parts.length > 2 ? parts[1] : '/';
			(groups[folder] ??= []).push(f);
		}
		return Object.entries(groups).sort(([a], [b]) =>
			a === '/' ? -1 : b === '/' ? 1 : a.localeCompare(b)
		);
	});

	function label(path: string) {
		return path.split('/').pop()!.replace(/\.md$/, '');
	}

	function openCreate() {
		draftFolder = '';
		draftName = '';
		createError = '';
		createOpen = true;
	}

	async function createPage(event: Event) {
		event.preventDefault();
		let name = draftName.trim().replace(/^\/+/, '');
		if (!name) return;
		if (!name.endsWith('.md')) name += '.md';
		const path = draftFolder ? `memory/${draftFolder}/${name}` : `memory/${name}`;
		creating = true;
		createError = '';
		try {
			await backend.syncFilePut(path, `# ${name.replace(/\.md$/, '')}\n`);
			createOpen = false;
			toast.success(`Created ${path}.`);
			goto(`/memory/${path.slice('memory/'.length)}`);
		} catch (e) {
			createError = e instanceof Error ? e.message : 'Could not create the page';
		} finally {
			creating = false;
		}
	}

	async function search(event: Event) {
		event.preventDefault();
		if (!query.trim()) return;
		searching = true;
		try {
			results = await backend.memorySearch(query);
		} catch (e) {
			results = [];
			toast.danger(e instanceof Error ? e.message : 'Search failed.');
		} finally {
			searching = false;
			searched = true;
		}
	}
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-wrap items-start justify-between gap-4">
		<div class="flex min-w-0 flex-col gap-2">
			<h1 class="text-fc-2xl font-semibold text-fc-fg">Memory</h1>
			<p class="text-fc-sm text-fc-fg-muted">
				Browse, search and curate the wiki your agents read from and write back to.
			</p>
		</div>
		<Button icon={icons.plus} onclick={openCreate}>New page</Button>
	</div>

	<section class="flex flex-col gap-4">
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
						<span class="truncate text-fc-sm text-fc-fg">{result.content}</span>
					</Card>
				{/each}
			</div>
		{:else if searched && !searching}
			<Card class="text-fc-sm text-fc-fg-muted">Nothing matched “{query}”.</Card>
		{/if}
	</section>

	{#if files.length === 0}
		<EmptyState
			icon={icons.folder}
			title="No memories yet"
			description="Create a page, or let your agents fill this in as they learn."
		>
			<Button variant="outline" icon={icons.plus} onclick={openCreate}>New page</Button>
		</EmptyState>
	{:else}
		{#each grouped as [folder, entries] (folder)}
			<section class="flex flex-col gap-4">
				<h2 class="flex items-center gap-2 text-fc-lg font-semibold text-fc-fg">
					<iconify-icon
						icon={icons.folder}
						width="18"
						height="18"
						class="block text-fc-fg-muted"
					></iconify-icon>
					{folder === '/' ? 'root' : folder}
				</h2>
				<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
					{#each entries as file (file.path)}
						<EntityCard
							href="/memory/{file.path.slice('memory/'.length)}"
							icon={icons.folder}
							title={label(file.path)}
							meta={file.path}
						/>
					{/each}
				</div>
			</section>
		{/each}
	{/if}
</div>

<Modal bind:open={createOpen} title="New memory page" showClose>
	<form class="flex flex-col gap-4" onsubmit={createPage}>
		<Field label="Folder" helper="Leave it on root for a page that fits nowhere else.">
			<Select bind:value={draftFolder} disabled={creating}>
				<option value="">root</option>
				{#each FOLDERS as folder (folder)}
					<option value={folder}>{folder}</option>
				{/each}
			</Select>
		</Field>
		<Field
			label="Name"
			helper="The .md extension is added if you leave it off."
			error={createError || undefined}
		>
			<Input bind:value={draftName} placeholder="dokploy-redeploy" disabled={creating} required />
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
				{creating ? 'Creating…' : 'Create page'}
			</Button>
		</div>
	</form>
</Modal>
