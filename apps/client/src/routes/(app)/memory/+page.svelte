<script lang="ts">
	import Icon from '@iconify/svelte';
	import { backend, type FileEntry } from '$lib/backend';

	let query = $state('');
	let results: { path: string; line: number; content: string }[] = $state([]);
	let searching = $state(false);

	let files: FileEntry[] = $state([]);
	let selected = $state('');
	let content = $state('');
	let loadingContent = $state(false);

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

	async function open(path: string) {
		selected = path;
		loadingContent = true;
		content = '';
		try {
			content = await backend.syncFile(path);
		} catch {
			content = '';
		} finally {
			loadingContent = false;
		}
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

<div class="space-y-6">
	<div>
		<h2 class="text-2xl font-semibold tracking-tight">Memory</h2>
		<p class="text-sm text-muted-foreground">Browse and search your shared agent memory.</p>
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
				<button onclick={() => open(r.path.startsWith('memory/') ? r.path : `memory/${r.path}`)} class="block w-full rounded-lg border border-border px-3 py-2 text-left hover:bg-accent">
					<span class="text-xs font-medium text-primary">{r.path}:{r.line}</span>
					<p class="text-sm">{r.content}</p>
				</button>
			{/each}
		</div>
	{/if}

	{#if files.length === 0}
		<div class="rounded-lg border border-dashed border-border p-8 text-center">
			<p class="text-sm text-muted-foreground">No memories yet. Your agents will fill this in as they learn.</p>
		</div>
	{:else}
		<div class="grid gap-4 md:grid-cols-[260px_1fr]">
			<nav class="space-y-3 md:max-h-[70vh] md:overflow-auto">
				{#each grouped as [folder, entries]}
					<div>
						<p class="mb-1 flex items-center gap-1.5 px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
							<Icon icon={folder === '/' ? 'solar:notebook-linear' : 'solar:folder-linear'} class="size-3.5" />
							{folder === '/' ? 'root' : folder}
						</p>
						{#each entries as f}
							<button
								onclick={() => open(f.path)}
								class="block w-full truncate rounded-md px-2 py-1 text-left text-sm transition-colors {selected === f.path ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'}"
							>
								{label(f.path)}
							</button>
						{/each}
					</div>
				{/each}
			</nav>

			<div class="min-w-0">
				{#if loadingContent}
					<p class="text-sm text-muted-foreground">Loading…</p>
				{:else if selected}
					<p class="mb-2 font-mono text-xs text-primary">{selected}</p>
					<pre class="max-h-[70vh] overflow-auto whitespace-pre-wrap rounded-lg border border-border bg-accent p-4 text-sm leading-relaxed">{content}</pre>
				{:else}
					<div class="flex h-full min-h-40 items-center justify-center rounded-lg border border-dashed border-border">
						<p class="text-sm text-muted-foreground">Select a page to read it.</p>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>
