<script lang="ts">
	import Icon from '@iconify/svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { backend } from '$lib/backend';

	let rel = $derived($page.params.path ?? '');
	let fullPath = $derived(`memory/${rel}`);
	let title = $derived(rel.split('/').pop()!.replace(/\.md$/, ''));

	let content = $state('');
	let draft = $state('');
	let editing = $state(false);
	let saving = $state(false);
	let loading = $state(true);

	$effect(() => {
		const p = fullPath;
		loading = true;
		editing = false;
		backend
			.syncFile(p)
			.then((c) => (content = c))
			.catch(() => (content = ''))
			.finally(() => (loading = false));
	});

	function edit() {
		draft = content;
		editing = true;
	}

	async function save() {
		saving = true;
		try {
			await backend.syncFilePut(fullPath, draft);
			content = draft;
			editing = false;
		} finally {
			saving = false;
		}
	}

	async function remove() {
		if (!confirm(`Delete memory page "${rel}"? This cannot be undone.`)) return;
		await backend.syncFileDelete(fullPath);
		goto('/memory');
	}
</script>

<div class="space-y-6">
	<a href="/memory" class="inline-flex items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground">
		<Icon icon="solar:alt-arrow-left-linear" class="size-4" />
		Memory
	</a>

	<div class="flex items-start justify-between gap-4">
		<div class="flex items-center gap-3">
			<div class="flex size-10 items-center justify-center rounded-xl bg-accent">
				<Icon icon="solar:document-text-linear" class="size-5 text-foreground" />
			</div>
			<div>
				<h2 class="text-xl font-semibold tracking-tight">{title}</h2>
				<p class="font-mono text-xs text-muted-foreground">{fullPath}</p>
			</div>
		</div>
		<div class="flex shrink-0 items-center gap-2">
			{#if editing}
				<button onclick={() => (editing = false)} class="rounded-lg border border-border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent">Cancel</button>
				<button onclick={save} disabled={saving} class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3.5 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50">
					<Icon icon="solar:diskette-linear" class="size-4" />
					{saving ? 'Saving…' : 'Save'}
				</button>
			{:else}
				<button onclick={remove} class="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-sm font-medium text-destructive transition-colors hover:bg-destructive/10">
					<Icon icon="solar:trash-bin-trash-linear" class="size-4" />
					Delete
				</button>
				<button onclick={edit} class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3.5 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90">
					<Icon icon="solar:pen-linear" class="size-4" />
					Edit
				</button>
			{/if}
		</div>
	</div>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{:else if editing}
		<textarea
			bind:value={draft}
			spellcheck="false"
			class="h-[62vh] w-full resize-none rounded-xl border border-input bg-background p-4 font-mono text-sm leading-relaxed outline-none focus-visible:ring-2 focus-visible:ring-ring"
		></textarea>
	{:else}
		<pre class="max-h-[62vh] overflow-auto whitespace-pre-wrap rounded-xl border border-border bg-accent/50 p-5 font-mono text-sm leading-relaxed">{content}</pre>
	{/if}
</div>
