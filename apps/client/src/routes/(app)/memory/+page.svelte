<script lang="ts">
	import Icon from '@iconify/svelte';
	import { goto } from '$app/navigation';
	import { backend, type FileEntry } from '$lib/backend';

	const FOLDERS = ['', 'bugs', 'tools', 'projects', 'conventions', 'syntheses'];

	let query = $state('');
	let results: { path: string; line: number; content: string }[] = $state([]);
	let searching = $state(false);

	let files: FileEntry[] = $state([]);

	$effect(() => {
		backend
			.syncTree()
			.then((t) => (files = t.filter((f) => f.path.startsWith('memory/')).sort((a, b) => a.path.localeCompare(b.path))))
			.catch(() => {});
	});

	let grouped = $derived.by(() => {
		const groups: Record<string, FileEntry[]> = {};
		for (const f of files) {
			const parts = f.path.split('/');
			const folder = parts.length > 2 ? parts[1] : '/';
			(groups[folder] ??= []).push(f);
		}
		return Object.entries(groups).sort(([a], [b]) => (a === '/' ? -1 : b === '/' ? 1 : a.localeCompare(b)));
	});

	function label(path: string) {
		return path.split('/').pop()!.replace(/\.md$/, '');
	}

	async function addPage() {
		const folder = prompt(`Folder (${FOLDERS.slice(1).join(', ')}, or leave empty for root):`, '');
		if (folder === null) return;
		const f = folder.trim().replace(/^\/+|\/+$/g, '');
		if (f && !FOLDERS.includes(f)) {
			alert(`Unknown folder "${f}". Use one of: ${FOLDERS.slice(1).join(', ')} or leave empty.`);
			return;
		}
		const rawName = prompt('Page name (e.g. my-finding):');
		if (!rawName) return;
		let name = rawName.trim().replace(/^\/+/, '');
		if (!name.endsWith('.md')) name += '.md';
		const path = f ? `memory/${f}/${name}` : `memory/${name}`;
		await backend.syncFilePut(path, `# ${name.replace(/\.md$/, '')}\n`);
		goto(`/memory/${path.slice('memory/'.length)}`);
	}

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

<div class="space-y-7">
	<div class="flex items-end justify-between gap-4">
		<div>
			<h2 class="text-2xl font-semibold tracking-tight">Memory</h2>
			<p class="mt-1 text-sm text-muted-foreground">Browse, search, and curate your shared agent memory.</p>
		</div>
		<button onclick={addPage} class="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-primary px-3.5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90">
			<Icon icon="solar:add-circle-linear" class="size-4" />
			New page
		</button>
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
				{@const rp = r.path.startsWith('memory/') ? r.path.slice('memory/'.length) : r.path}
				<a href="/memory/{rp}" class="block w-full rounded-lg border border-border px-3 py-2 text-left hover:bg-accent">
					<span class="text-xs font-medium text-primary">{r.path}:{r.line}</span>
					<p class="text-sm">{r.content}</p>
				</a>
			{/each}
		</div>
	{/if}

	{#if files.length === 0}
		<div class="rounded-xl border border-dashed border-border p-12 text-center">
			<Icon icon="solar:notebook-linear" class="mx-auto size-6 text-muted-foreground/50" />
			<p class="mt-2 text-sm text-muted-foreground">No memories yet. Create one or let your agents fill this in as they learn.</p>
		</div>
	{:else}
		<div class="space-y-6">
			{#each grouped as [folder, entries]}
				<div>
					<p class="mb-2 flex items-center gap-1.5 px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
						<Icon icon={folder === '/' ? 'solar:notebook-linear' : 'solar:folder-linear'} class="size-3.5" />
						{folder === '/' ? 'root' : folder}
					</p>
					<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
						{#each entries as f}
							<a
								href="/memory/{f.path.slice('memory/'.length)}"
								class="group rounded-xl border border-border bg-background p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-sm"
							>
								<div class="flex items-center justify-between">
									<div class="flex size-9 items-center justify-center rounded-lg bg-accent">
										<Icon icon="solar:document-text-linear" class="size-[18px] text-foreground" />
									</div>
									<Icon icon="solar:alt-arrow-right-linear" class="size-4 text-muted-foreground/30 transition-all group-hover:translate-x-0.5 group-hover:text-muted-foreground" />
								</div>
								<p class="mt-3 truncate font-medium">{label(f.path)}</p>
								<p class="truncate font-mono text-xs text-muted-foreground">{f.path}</p>
							</a>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
