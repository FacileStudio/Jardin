<script lang="ts">
	import { Alert, Badge, Card, EmptyState, Input, Spinner, icons, toast } from '@facile/muse';
	import { backend, type ReportSummary } from '$lib/backend';

	let reports: ReportSummary[] = $state([]);
	let loading = $state(true);
	let error = $state('');
	let query = $state('');

	$effect(() => {
		loading = true;
		backend
			.reportsList()
			.then((r) => (reports = r))
			.catch((e) => (error = e instanceof Error ? e.message : 'Could not load reports.'))
			.finally(() => (loading = false));
	});

	const filtered = $derived(
		reports.filter((r) => {
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

	function formatExpiry(rep: ReportSummary): string {
		if (!rep.expires) return 'Pinned';
		if (rep.expired) return 'Expired';
		try {
			const exp = new Date(rep.expires).getTime();
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
		<h1 class="text-fc-2xl font-bold text-fc-fg">Reports</h1>
		<p class="text-fc-sm text-fc-fg-muted">
			Rendered pages and documents generated across your fleet, synced for browser viewing.
		</p>
	</div>

	{#if loading}
		<div class="flex items-center gap-3 text-fc-sm text-fc-fg-muted">
			<Spinner size="sm" /> Loading reports…
		</div>
	{:else if error}
		<Alert tone="danger" title="Could not load reports">{error}</Alert>
	{:else if reports.length === 0}
		<EmptyState
			icon={icons.card}
			title="No reports yet"
			description="Publish one with `mycelium report add <file>` or the `publish_report` tool."
		/>
	{:else}
		<div class="flex flex-col gap-4">
			<div class="max-w-md">
				<Input
					type="search"
					placeholder="Filter reports by title, machine…"
					bind:value={query}
				/>
			</div>

			{#if filtered.length === 0}
				<p class="py-8 text-center text-fc-sm text-fc-fg-muted">
					No reports match “{query}”.
				</p>
			{:else}
				<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
					{#each filtered as rep (rep.id)}
						<a
							href="/reports/{rep.id}"
							class="group flex flex-col justify-between rounded-fc-lg border border-fc-border bg-fc-surface p-5 transition-all hover:border-fc-ring/40 hover:bg-fc-surface-hover hover:shadow-fc-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-fc-ring"
						>
							<div class="flex flex-col gap-3">
								<div class="flex items-start justify-between gap-2">
									<h2 class="truncate text-fc-base font-semibold text-fc-fg transition-colors group-hover:text-fc-primary">
										{rep.title}
									</h2>
									<Badge tone={rep.expired ? 'danger' : rep.expires ? 'neutral' : 'success'}>
										{formatExpiry(rep)}
									</Badge>
								</div>
								<p class="font-fc-mono text-fc-xs text-fc-fg-muted truncate">
									reports/{rep.id}.html
								</p>
							</div>

							<div class="mt-4 flex items-center justify-between border-t border-fc-border/60 pt-3 text-fc-xs text-fc-fg-muted">
								<span class="inline-flex items-center gap-1 font-fc-mono">
									<iconify-icon icon={icons.server} width="12" height="12"></iconify-icon>
									{rep.machine || 'unknown'}
								</span>
								<span>{formatDate(rep.created)}</span>
							</div>
						</a>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
