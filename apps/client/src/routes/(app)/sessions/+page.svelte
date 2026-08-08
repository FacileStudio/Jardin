<script lang="ts">
	import {
		BarChart,
		Card,
		DonutChart,
		EmptyState,
		LineChart,
		Sparkline,
		StatCard,
		StatusDot,
		Table,
		Tabs,
		icons,
		type ChartSeries
	} from '@facile/muse';
	import {
		backend,
		type LiveSession,
		type SessionBlock,
		type SessionStats,
		type SessionTimeline,
		type UsageHistory,
		type UsageSnapshot
	} from '$lib/backend';
	import UsageMeter from '$lib/components/UsageMeter.svelte';
	import {
		bucketLabel,
		columnTotals,
		formatDuration,
		formatTokens,
		hours,
		periodDelta
	} from '$lib/metrics';

	const RANGES = [
		{ id: '7d', label: '7 days' },
		{ id: '30d', label: '30 days' },
		{ id: 'all', label: 'All time' }
	];
	const GROUPS = [
		{ id: 'project', label: 'Project', icon: icons.folder },
		{ id: 'machine', label: 'Machine', icon: icons.server },
		{ id: 'agent', label: 'Agent', icon: icons.bolt },
		{ id: 'model', label: 'Model', icon: icons.dashboard }
	];

	/* Eight bars is where a horizontal chart stops being readable in a card; the table below
	   still lists every row, so nothing is hidden. */
	const CHART_ROWS = 8;

	let since = $state('7d');
	let by = $state('project');
	let stats: SessionStats | null = $state(null);
	let recent: SessionBlock[] = $state([]);
	let live: LiveSession[] = $state([]);
	let timeline: SessionTimeline | null = $state(null);
	let usage: UsageSnapshot[] = $state([]);
	let usageLog: UsageHistory | null = $state(null);

	/* All time over daily buckets is a thousand pixels of noise; months keep the axis honest. */
	const bucket = $derived(since === 'all' ? 'month' : 'day');
	const bucketUnit = $derived(bucket === 'month' ? 'month' : 'day');

	$effect(() => {
		const load = () =>
			backend
				.sessionsLive()
				.then((l) => (live = l ?? []))
				.catch(() => {});
		load();
		const timer = setInterval(load, 30_000);
		return () => clearInterval(timer);
	});

	$effect(() => {
		backend
			.sessionsStats(since, by)
			.then((s) => (stats = s))
			.catch(() => {});
	});

	$effect(() => {
		backend
			.sessionsRecent(15)
			.then((r) => (recent = r ?? []))
			.catch(() => {});
	});

	$effect(() => {
		backend
			.sessionsTimeline(since, bucket, by)
			.then((t) => (timeline = t))
			.catch(() => (timeline = null));
	});

	$effect(() => {
		const load = () => {
			backend
				.usageCurrent()
				.then((u) => (usage = u ?? []))
				.catch(() => (usage = []));
			backend
				.usageHistory('7d')
				.then((h) => (usageLog = h))
				.catch(() => (usageLog = null));
		};
		load();
		const timer = setInterval(load, 60_000);
		return () => clearInterval(timer);
	});

	/* $derived.by so `stats` reads as its declared type rather than the null it was
	   initialised with — see the same note in (app)/+layout.svelte. */
	const rows = $derived.by(() => stats?.rows ?? []);
	const totalSeconds = $derived(rows.reduce((sum, r) => sum + r.seconds, 0));
	const totalSessions = $derived(rows.reduce((sum, r) => sum + r.sessions, 0));
	const totalTokensOut = $derived(rows.reduce((sum, r) => sum + r.tokens_out, 0));

	const totalCacheRead = $derived(rows.reduce((sum, r) => sum + r.cache_read, 0));

	const ranked = $derived([...rows].sort((a, b) => b.seconds - a.seconds).slice(0, CHART_ROWS));
	const chartSeries = $derived([{ name: 'Active time', data: ranked.map((r) => hours(r.seconds)) }]);
	const chartLabels = $derived(ranked.map((r) => r.key));

	/* The donut keeps to muse's six-slot ceiling; the table below still lists every row. */
	const shareSlices = $derived(
		[...rows]
			.sort((a, b) => b.seconds - a.seconds)
			.slice(0, 6)
			.map((r) => ({ label: r.key, value: hours(r.seconds) }))
	);

	const labels = $derived.by(() => timeline?.labels ?? []);
	const tSeries = $derived.by(() => timeline?.series ?? []);
	const trendSeries: ChartSeries[] = $derived(
		tSeries.map((s) => ({ name: s.key, data: s.seconds.map(hours) }))
	);
	const secondsPerBucket = $derived(columnTotals(tSeries.map((s) => s.seconds)));
	const sessionsPerBucket = $derived(columnTotals(tSeries.map((s) => s.sessions)));
	const tokensPerBucket = $derived(columnTotals(tSeries.map((s) => s.tokens_out)));
	const cachePerBucket = $derived(columnTotals(tSeries.map((s) => s.cache_read)));
	const hasTimeline = $derived(labels.length > 0 && tSeries.length > 0);

	const timeDelta = $derived(periodDelta(secondsPerBucket, bucketUnit));
	const sessionsDelta = $derived(periodDelta(sessionsPerBucket, bucketUnit));
	const tokensDelta = $derived(periodDelta(tokensPerBucket, bucketUnit));
	const cacheDelta = $derived(periodDelta(cachePerBucket, bucketUnit));

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

	/* Three states, not a boolean: a machine that went offline, an agent thinking, and an
	   agent that has been idle for twenty minutes all need different reactions. */
	function liveState(s: LiveSession): 'active' | 'idle' | 'offline' {
		if (!s.machine_online) return 'offline';
		return s.live ? 'active' : 'idle';
	}

	const liveTones = { active: 'success', idle: 'warning', offline: 'neutral' } as const;

	function liveLabel(s: LiveSession): string {
		const state = liveState(s);
		if (state === 'offline') return 'machine offline';
		if (state === 'active') return 'active';
		return `idle ${Math.max(1, Math.round(s.idle_seconds / 60))}m`;
	}

	function blockDuration(b: SessionBlock): string {
		const seconds = Math.max(
			0,
			(new Date(b.ended_at).getTime() - new Date(b.started_at).getTime()) / 1000
		);
		return formatDuration(seconds);
	}
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-col gap-2">
		<h1 class="text-fc-2xl font-semibold text-fc-fg">Sessions</h1>
		<p class="text-fc-sm text-fc-fg-muted">
			Agent work sessions recorded across every machine that syncs with this brain.
		</p>
	</div>

	<section class="flex flex-col gap-4">
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div class="flex min-w-0 flex-col gap-1">
				<h2 class="text-fc-lg font-semibold text-fc-fg">Running now</h2>
				<p class="text-fc-sm text-fc-fg-muted">Refreshed every thirty seconds.</p>
			</div>
			<!-- The phone's nav bar only holds four destinations, so this is how Machines is
			     reached from a phone. -->
			<a
				href="/machines"
				class="inline-flex shrink-0 items-center gap-1 text-fc-sm text-fc-fg-muted transition-colors hover:text-fc-fg focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-fc-ring"
			>
				<iconify-icon icon={icons.server} width="16" height="16" class="block"></iconify-icon>
				Machines
			</a>
		</div>

		{#if live.length === 0}
			<Card class="text-fc-sm text-fc-fg-muted">No session is running.</Card>
		{:else}
			<div class="flex flex-col gap-2">
				{#each live as session (session.machine + session.project + session.started_at)}
					<Card
						class="flex flex-wrap items-center gap-x-4 gap-y-2 py-3 {session.machine_online
							? ''
							: 'opacity-60'}"
					>
						<StatusDot
							tone={liveTones[liveState(session)]}
							label={liveLabel(session)}
							pulse={liveState(session) === 'active'}
						/>
						<span class="text-fc-sm font-medium text-fc-fg">{session.project}</span>
						<span class="font-fc-mono text-fc-xs text-fc-fg-muted">
							{session.machine}/{session.agent}
						</span>
						{#if session.branch}
							<span
								class="rounded-fc-xs bg-fc-surface px-1.5 py-0.5 font-fc-mono text-fc-xs text-fc-fg-muted"
							>
								{session.branch}
							</span>
						{/if}
						<span class="ml-auto text-fc-xs tabular-nums text-fc-fg-muted">
							{elapsed(session.started_at)} · {formatTokens(session.tokens_out)} out
						</span>
					</Card>
				{/each}
			</div>
		{/if}
	</section>

	<section class="flex flex-col gap-4">
		<div class="flex flex-col gap-1">
			<h2 class="text-fc-lg font-semibold text-fc-fg">Totals</h2>
			<p class="text-fc-sm text-fc-fg-muted">
				Sealed sessions only — whatever is running now lands here when it ends.
			</p>
		</div>

		<div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
			<Tabs items={RANGES} bind:value={since} label="Time range" />
			<Tabs items={GROUPS} bind:value={by} label="Group sessions by" />
		</div>

		{#if rows.length === 0}
			<EmptyState
				icon={icons.history}
				title="No sessions recorded yet"
				description="Machines record agent activity automatically once they run mycelium v0.5 or later."
			/>
		{:else}
			<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
				<StatCard label="Active time" value={formatDuration(totalSeconds)} delta={timeDelta}>
					<Sparkline data={secondsPerBucket.map(hours)} class="mt-3" showLast />
				</StatCard>
				<StatCard label="Sessions" value={totalSessions} delta={sessionsDelta}>
					<Sparkline
						data={sessionsPerBucket}
						class="mt-3"
						color="var(--color-fc-chart-3)"
						valueFormat={(n) => `${n}`}
					/>
				</StatCard>
				<StatCard label="Tokens out" value={formatTokens(totalTokensOut)} delta={tokensDelta}>
					<Sparkline data={tokensPerBucket} class="mt-3" color="var(--color-fc-chart-2)" />
				</StatCard>
				<StatCard label="Cache read" value={formatTokens(totalCacheRead)} delta={cacheDelta}>
					<Sparkline data={cachePerBucket} class="mt-3" color="var(--color-fc-chart-5)" />
				</StatCard>
			</div>

			{#if hasTimeline}
				<div class="grid gap-4 lg:grid-cols-3">
					<Card class="flex flex-col gap-4 lg:col-span-2">
						<p class="text-fc-sm font-medium text-fc-fg">
							Active time per {bucketUnit}, by {by}
						</p>
						<LineChart
							series={trendSeries}
							{labels}
							area
							height={240}
							class="flex-1"
							yFormat={(n) => `${n} h`}
							xFormat={(l) => bucketLabel(l)}
						/>
					</Card>
					<Card class="flex flex-col gap-4">
						<p class="text-fc-sm font-medium text-fc-fg">Share by {by}</p>
						<DonutChart
							data={shareSlices}
							centerLabel="Active"
							centerValue={formatDuration(totalSeconds)}
							valueFormat={(n) => `${n} h`}
							class="flex-1"
						/>
					</Card>
				</div>
			{/if}

			<Card class="flex flex-col gap-4">
				<p class="text-fc-sm font-medium text-fc-fg">
					Active time by {by}
				</p>
				<BarChart
					series={chartSeries}
					labels={chartLabels}
					horizontal
					height={240}
					yFormat={(n) => `${n} h`}
				/>
			</Card>

			<Table>
				<thead>
					<tr>
						<th scope="col">{GROUPS.find((g) => g.id === by)?.label ?? by}</th>
						<th scope="col" class="text-right">Sessions</th>
						<th scope="col" class="text-right">Active</th>
						<th scope="col" class="text-right">Tokens in</th>
						<th scope="col" class="text-right">Tokens out</th>
					</tr>
				</thead>
				<tbody>
					{#each rows as row (row.key)}
						<tr>
							<td class="font-medium text-fc-fg">{row.key}</td>
							<td class="text-right tabular-nums">{row.sessions}</td>
							<td class="text-right tabular-nums">{formatDuration(row.seconds)}</td>
							<td class="text-right tabular-nums">{formatTokens(row.tokens_in)}</td>
							<td class="text-right tabular-nums">{formatTokens(row.tokens_out)}</td>
						</tr>
					{/each}
				</tbody>
			</Table>
		{/if}
	</section>

	<section class="flex flex-col gap-4">
		<div class="flex flex-col gap-1">
			<h2 class="text-fc-lg font-semibold text-fc-fg">Plan usage</h2>
			<p class="text-fc-sm text-fc-fg-muted">
				How much of each Claude subscription window this machine has burned through.
			</p>
		</div>
		<UsageMeter snapshots={usage} history={usageLog} />
	</section>

	{#if recent.length > 0}
		<section class="flex flex-col gap-4">
			<h2 class="text-fc-lg font-semibold text-fc-fg">Recent</h2>
			<Table>
				<thead>
					<tr>
						<th scope="col">Ended</th>
						<th scope="col">Project</th>
						<th scope="col">Machine / agent</th>
						<th scope="col" class="text-right">Duration</th>
						<th scope="col" class="text-right">Tokens out</th>
					</tr>
				</thead>
				<tbody>
					{#each recent as block (block.id)}
						<tr>
							<td class="whitespace-nowrap tabular-nums text-fc-fg-muted">
								{formatEnded(block.ended_at)}
							</td>
							<td class="font-medium text-fc-fg">{block.project}</td>
							<td class="whitespace-nowrap font-fc-mono text-fc-xs text-fc-fg-muted">
								{block.machine}/{block.agent}{block.branch ? ` · ${block.branch}` : ''}
							</td>
							<td class="text-right tabular-nums">{blockDuration(block)}</td>
							<td class="text-right tabular-nums">{formatTokens(block.tokens_out)}</td>
						</tr>
					{/each}
				</tbody>
			</Table>
		</section>
	{/if}
</div>
