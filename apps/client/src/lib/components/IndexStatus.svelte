<script lang="ts">
	import { Badge, Card, icons } from '@facile/muse';
	import { backend, type MemoryIndexState } from '$lib/backend';

	/*
	 * Two cadences, because the two states answer different questions. A run in flight is a
	 * moving number a human watches, so it refreshes every 2s; a finished index only changes
	 * when someone writes a page, and 30s is soon enough to notice. Polling stops entirely on
	 * a hidden tab — a dashboard left open in a background tab is the whole reason this file
	 * has a visibility listener rather than a bare interval.
	 */
	const LIVE_MS = 2_000;
	const IDLE_MS = 30_000;

	let index: MemoryIndexState | null = $state(null);
	let error = $state('');
	let inFlight = false;
	let visible = $state(true);

	async function load() {
		if (inFlight) return;
		inFlight = true;
		try {
			index = await backend.memoryIndexStatus();
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not read the index state.';
		} finally {
			inFlight = false;
		}
	}

	$effect(() => {
		const sync = () => (visible = !document.hidden);
		sync();
		document.addEventListener('visibilitychange', sync);
		return () => document.removeEventListener('visibilitychange', sync);
	});

	const period = $derived.by(() => (index?.indexing ? LIVE_MS : IDLE_MS));

	$effect(() => {
		if (!visible) return;
		const every = period;
		load();
		const timer = setInterval(load, every);
		return () => clearInterval(timer);
	});

	const percent = $derived.by(() => {
		if (!index || index.total_chunks <= 0) return 0;
		return Math.max(0, Math.min(100, Math.round((index.indexed_chunks / index.total_chunks) * 100)));
	});

	function count(n: number): string {
		return n.toLocaleString('en-US');
	}

	function rateText(rate: number): string {
		if (!Number.isFinite(rate) || rate <= 0) return 'measuring rate…';
		return `${rate.toFixed(rate >= 10 ? 0 : 2)} chunks/sec`;
	}

	/*
	 * An ETA computed from a rate is an estimate, and it should read like one: rounded to
	 * something a human can act on, never to the second the arithmetic produced.
	 */
	function etaText(seconds: number): string {
		if (!Number.isFinite(seconds) || seconds <= 0) return 'almost done';
		if (seconds < 60) return 'less than a minute left';
		const minutes = Math.round(seconds / 60);
		if (minutes < 60) return `about ${minutes} minute${minutes === 1 ? '' : 's'} left`;
		const h = Math.floor(minutes / 60);
		const m = minutes % 60;
		const head = `about ${h} hour${h === 1 ? '' : 's'}`;
		return m === 0 ? `${head} left` : `${head} ${m} min left`;
	}
</script>

{#if index || error}
	<Card class="flex flex-col gap-3">
		<div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
			<span class="flex min-w-0 items-center gap-2">
				<iconify-icon
					icon={icons.search}
					width="16"
					height="16"
					class="block shrink-0 text-fc-fg-muted"
				></iconify-icon>
				<span class="text-fc-sm font-medium text-fc-fg">Semantic index</span>
			</span>
			{#if index?.indexing}
				<Badge tone="info">Indexing</Badge>
			{:else if index?.enabled}
				<Badge tone="success">Ready</Badge>
			{:else if index}
				<Badge tone="neutral">Off</Badge>
			{/if}
		</div>

		{#if index && !index.enabled}
			<p class="text-fc-sm text-fc-fg-muted">
				Semantic search is off. Search matches words literally until a model is configured.
			</p>
		{:else if index}
			{#if index.indexing}
				<div class="flex flex-col gap-1.5">
					<div class="flex items-baseline justify-between gap-3">
						<span class="truncate text-fc-sm text-fc-fg">
							{count(index.indexed_chunks)} / {count(index.total_chunks)} chunks
						</span>
						<span class="shrink-0 text-fc-sm font-medium tabular-nums text-fc-fg">{percent}%</span>
					</div>
					<div
						class="h-2 w-full overflow-hidden rounded-fc-full bg-fc-surface"
						role="progressbar"
						aria-valuenow={percent}
						aria-valuemin={0}
						aria-valuemax={100}
						aria-label="Chunks embedded"
					>
						<div
							class="h-full rounded-fc-full bg-fc-accent transition-[width] duration-500"
							style:width="{percent}%"
						></div>
					</div>
					<p class="text-fc-xs text-fc-fg-muted">
						{rateText(index.chunks_per_second)} · {etaText(index.eta_seconds)}{index.pending_paths >
						0
							? ` · ${count(index.pending_paths)} page${index.pending_paths === 1 ? '' : 's'} queued`
							: ''}
					</p>
				</div>
			{:else}
				<p class="text-fc-sm text-fc-fg-muted">
					{count(index.indexed_chunks)} chunks embedded{index.pending_paths > 0
						? `, ${count(index.pending_paths)} page${index.pending_paths === 1 ? '' : 's'} queued`
						: ''}.
				</p>
			{/if}

			<div class="flex flex-wrap items-center gap-x-3 gap-y-1 font-fc-mono text-fc-xs text-fc-fg-muted">
				{#if index.model?.name}
					<span class="truncate">{index.model.name}</span>
				{/if}
				{#if index.model?.dims}
					<span>{index.model.dims} dims</span>
				{/if}
				{#if index.store}
					<span>{index.store} store</span>
				{/if}
			</div>
		{/if}

		{#if index?.last_error || error}
			<p
				class="flex items-start gap-2 rounded-fc-sm bg-fc-danger/10 px-3 py-2 text-fc-xs text-fc-danger"
			>
				<iconify-icon
					icon={icons.warning}
					width="14"
					height="14"
					class="mt-0.5 block shrink-0"
				></iconify-icon>
				<span class="min-w-0 break-words">{index?.last_error || error}</span>
			</p>
		{/if}
	</Card>
{/if}
