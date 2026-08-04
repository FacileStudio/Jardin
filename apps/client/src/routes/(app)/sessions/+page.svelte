<script lang="ts">
	import Icon from '@iconify/svelte';
	import { backend, type LiveSession, type SessionBlock, type SessionStats } from '$lib/backend';

	const RANGES = [
		{ label: '7d', value: '7d' },
		{ label: '30d', value: '30d' },
		{ label: 'all', value: 'all' }
	];
	const GROUPS = [
		{ label: 'Project', value: 'project' },
		{ label: 'Machine', value: 'machine' },
		{ label: 'Agent', value: 'agent' }
	];

	let since = $state('7d');
	let by = $state('project');
	let stats: SessionStats | null = $state(null);
	let recent: SessionBlock[] = $state([]);
	let live: LiveSession[] = $state([]);

	$effect(() => {
		const load = () => backend.sessionsLive().then((l) => (live = l ?? [])).catch(() => {});
		load();
		const timer = setInterval(load, 30_000);
		return () => clearInterval(timer);
	});

	$effect(() => {
		backend.sessionsStats(since, by).then((s) => (stats = s)).catch(() => {});
	});

	$effect(() => {
		backend.sessionsRecent(15).then((r) => (recent = r ?? [])).catch(() => {});
	});

	let rows = $derived.by(() => stats?.rows ?? []);
	let totalSeconds = $derived(rows.reduce((sum, r) => sum + r.seconds, 0));
	let totalSessions = $derived(rows.reduce((sum, r) => sum + r.sessions, 0));
	let totalTokensOut = $derived(rows.reduce((sum, r) => sum + r.tokens_out, 0));

	function formatDuration(seconds: number): string {
		const h = Math.floor(seconds / 3600);
		const m = Math.round((seconds % 3600) / 60);
		if (h > 0) return `${h}h${String(m).padStart(2, '0')}m`;
		return `${m}m`;
	}

	function formatTokens(n: number): string {
		if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
		if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
		return `${n}`;
	}

	function formatEnded(iso: string): string {
		const d = new Date(iso);
		const month = d.toLocaleString('en-US', { month: 'short' });
		const day = String(d.getDate()).padStart(2, '0');
		const hh = String(d.getHours()).padStart(2, '0');
		const mm = String(d.getMinutes()).padStart(2, '0');
		return `${month} ${day} ${hh}:${mm}`;
	}

	function elapsed(iso: string): string {
		return formatDuration(Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000));
	}

	function liveState(s: LiveSession): 'active' | 'idle' | 'offline' {
		if (!s.machine_online) return 'offline';
		return s.live ? 'active' : 'idle';
	}

	function liveDot(s: LiveSession): string {
		const state = liveState(s);
		if (state === 'offline') return 'bg-muted-foreground/40';
		if (state === 'idle') return 'bg-amber-500';
		return 'bg-emerald-500 animate-pulse';
	}

	function liveLabel(s: LiveSession): string {
		const state = liveState(s);
		if (state === 'offline') return 'machine offline';
		if (state === 'active') return 'active';
		return `idle ${Math.max(1, Math.round(s.idle_seconds / 60))}m`;
	}

	function blockDuration(b: SessionBlock): string {
		const seconds = Math.max(0, (new Date(b.ended_at).getTime() - new Date(b.started_at).getTime()) / 1000);
		return formatDuration(seconds);
	}
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-2xl font-semibold tracking-tight">Sessions</h2>
		<p class="text-sm text-muted-foreground">Agent work sessions recorded across your machines.</p>
	</div>

	<section class="space-y-3">
		<h3 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Running now</h3>
		{#if live.length === 0}
			<p class="text-sm text-muted-foreground">No sessions running.</p>
		{:else}
			<div class="space-y-1.5">
				{#each live as s}
					<div
						class="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-border px-4 py-2.5 text-sm {s.machine_online
							? ''
							: 'opacity-60'}"
					>
						<span class="size-2 flex-shrink-0 rounded-full {liveDot(s)}"></span>
						<span class="font-medium">{s.project}</span>
						<span class="text-muted-foreground">{s.machine}/{s.agent}</span>
						{#if s.branch}
							<code class="rounded bg-accent px-1 py-0.5 text-xs">{s.branch}</code>
						{/if}
						<span class="tabular-nums text-muted-foreground">{elapsed(s.started_at)}</span>
						<span class="tabular-nums text-muted-foreground">{formatTokens(s.tokens_out)} out</span>
						<span class="ml-auto text-muted-foreground">{liveLabel(s)}</span>
					</div>
				{/each}
			</div>
		{/if}
	</section>

	<div class="flex flex-wrap items-center gap-4">
		<div class="flex gap-1.5">
			{#each RANGES as r}
				<button
					onclick={() => (since = r.value)}
					class="rounded-lg border border-border px-3 py-1.5 text-sm transition-colors hover:bg-accent {since === r.value ? 'bg-accent font-medium text-foreground' : 'font-medium'}"
				>
					{r.label}
				</button>
			{/each}
		</div>
		<div class="flex gap-1.5">
			{#each GROUPS as g}
				<button
					onclick={() => (by = g.value)}
					class="rounded-lg border border-border px-3 py-1.5 text-sm transition-colors hover:bg-accent {by === g.value ? 'bg-accent font-medium text-foreground' : 'font-medium'}"
				>
					{g.label}
				</button>
			{/each}
		</div>
	</div>

	{#if rows.length === 0}
		<div class="rounded-xl border border-dashed border-border p-12 text-center">
			<Icon icon="solar:history-linear" class="mx-auto size-6 text-muted-foreground/50" />
			<p class="mt-2 text-sm text-muted-foreground">
				No sessions yet — machines record agent activity automatically once they run jardin v0.5+.
			</p>
		</div>
	{:else}
		<div class="grid gap-3 sm:grid-cols-3">
			<div class="rounded-xl border border-border bg-background p-5">
				<p class="text-xs uppercase tracking-wide text-muted-foreground">Active time</p>
				<p class="mt-1 text-2xl font-semibold tabular-nums">{formatDuration(totalSeconds)}</p>
			</div>
			<div class="rounded-xl border border-border bg-background p-5">
				<p class="text-xs uppercase tracking-wide text-muted-foreground">Sessions</p>
				<p class="mt-1 text-2xl font-semibold tabular-nums">{totalSessions}</p>
			</div>
			<div class="rounded-xl border border-border bg-background p-5">
				<p class="text-xs uppercase tracking-wide text-muted-foreground">Tokens out</p>
				<p class="mt-1 text-2xl font-semibold tabular-nums">{formatTokens(totalTokensOut)}</p>
			</div>
		</div>

		<div class="overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
						<th class="px-2 py-2 font-semibold">Key</th>
						<th class="px-2 py-2 text-right font-semibold">Sessions</th>
						<th class="px-2 py-2 text-right font-semibold">Active</th>
						<th class="px-2 py-2 text-right font-semibold">Tokens in</th>
						<th class="px-2 py-2 text-right font-semibold">Tokens out</th>
					</tr>
				</thead>
				<tbody>
					{#each rows as row}
						<tr class="border-b border-border/50">
							<td class="px-2 py-2 font-medium">{row.key}</td>
							<td class="px-2 py-2 text-right tabular-nums">{row.sessions}</td>
							<td class="px-2 py-2 text-right tabular-nums">{formatDuration(row.seconds)}</td>
							<td class="px-2 py-2 text-right tabular-nums">{formatTokens(row.tokens_in)}</td>
							<td class="px-2 py-2 text-right tabular-nums">{formatTokens(row.tokens_out)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}

	{#if recent.length > 0}
		<section class="space-y-3">
			<h3 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Recent</h3>
			<div class="space-y-1.5">
				{#each recent as b}
					<div class="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-border px-4 py-2.5 text-sm">
						<span class="tabular-nums text-muted-foreground">{formatEnded(b.ended_at)}</span>
						<span class="font-medium">{b.project}</span>
						<span class="text-muted-foreground">{b.machine}/{b.agent}</span>
						{#if b.branch}
							<code class="rounded bg-accent px-1 py-0.5 text-xs">{b.branch}</code>
						{/if}
						<span class="ml-auto tabular-nums text-muted-foreground">{blockDuration(b)}</span>
						<span class="tabular-nums text-muted-foreground">{formatTokens(b.tokens_out)} out</span>
					</div>
				{/each}
			</div>
		</section>
	{/if}
</div>
