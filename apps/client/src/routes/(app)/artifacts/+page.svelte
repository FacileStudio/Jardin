<script lang="ts">
	import { Alert, Badge, EmptyState, Input, Spinner, icons } from '@facile/muse';
	import { backend, type ArtifactSummary } from '$lib/backend';

	let artifacts: ArtifactSummary[] = $state([]);
	let loading = $state(true);
	let error = $state('');
	let query = $state('');

	$effect(() => {
		loading = true;
		backend
			.artifactsList()
			.then((r) => (artifacts = r ?? []))
			.catch((e) => (error = e instanceof Error ? e.message : 'Could not load artifacts.'))
			.finally(() => (loading = false));
	});

	const filtered = $derived(
		(artifacts ?? []).filter((r) => {
			const q = query.trim().toLowerCase();
			if (!q) return true;
			return r.title.toLowerCase().includes(q) || r.machine.toLowerCase().includes(q) || r.id.toLowerCase().includes(q);
		})
	);

	function formatDate(iso: string): string {
		try {
			const d = new Date(iso);
			return d.toLocaleDateString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}

	function formatExpiry(art: ArtifactSummary): string {
		if (!art.expires) return 'Pinned';
		if (art.expired) return 'Expired';
		try {
			const exp = new Date(art.expires).getTime();
			const now = Date.now();
			const diffDays = Math.ceil((exp - now) / (1000 * 60 * 60 * 24));
			if (diffDays <= 0) return 'Expiring today';
			if (diffDays === 1) return 'Expires tomorrow';
			return `Expires in ${diffDays}d`;
		} catch {
			return 'Temporary';
		}
	}
</script>

<div class="flex flex-col gap-8">
	<div class="flex flex-col gap-2">
		<h1 class="text-fc-2xl font-bold text-fc-fg">Artifacts & Reports</h1>
		<p class="text-fc-sm text-fc-fg-muted">
			Rendered markdown documents, reports, and architecture blueprints generated across your fleet.
		</p>
	</div>

	{#if loading}
		<div class="flex items-center gap-3 text-fc-sm text-fc-fg-muted">
			<Spinner size="sm" label="Loading" /> Loading artifacts…
		</div>
	{:else if error}
		<Alert tone="danger" title="Could not load artifacts">{error}</Alert>
	{:else if artifacts.length === 0}
		<EmptyState
			icon="solar:file-smile-linear"
			title="No artifacts yet"
			description="Publish one with `mycelium artifact add <file>` or the `publish_artifact` tool."
		/>
	{:else}
		<div class="flex flex-col gap-4">
			<div class="max-w-md">
				<Input
					type="search"
					placeholder="Filter artifacts by title, machine…"
					bind:value={query}
				/>
			</div>

			{#if filtered.length === 0}
				<p class="py-8 text-center text-fc-sm text-fc-fg-muted">
					No artifacts match “{query}”.
				</p>
			{:else}
				<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
					{#each filtered as art (art.id)}
						<a
							href="/artifacts/{art.id}"
							class="group flex flex-col justify-between rounded-fc-lg border border-fc-border bg-fc-surface p-5 transition-all hover:border-fc-ring/40 hover:bg-fc-surface-hover hover:shadow-fc-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-fc-ring"
						>
							<div class="flex flex-col gap-3">
								<div class="flex items-start justify-between gap-2">
									<h2 class="truncate text-fc-base font-semibold text-fc-fg transition-colors group-hover:text-fc-primary">
										{art.title}
									</h2>
									<div class="flex items-center gap-1.5 shrink-0">
										<Badge tone={art.expired ? 'danger' : art.expires ? 'neutral' : 'success'}>
											{formatExpiry(art)}
										</Badge>
										<Badge tone="neutral">
											{art.format === 'html' ? 'HTML' : 'MD'}
										</Badge>
									</div>
								</div>
								<p class="font-fc-mono text-fc-xs text-fc-fg-muted truncate">
									artifacts/{art.id}.{art.format === 'html' ? 'html' : 'md'}
								</p>
							</div>

							<div class="mt-4 flex items-center justify-between border-t border-fc-border/60 pt-3 text-fc-xs text-fc-fg-muted">
								<span class="inline-flex items-center gap-1 font-fc-mono">
									<iconify-icon icon={icons.server} width="12" height="12"></iconify-icon>
									{art.machine || 'unknown'}
								</span>
								<span>{formatDate(art.created)}</span>
							</div>
						</a>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
