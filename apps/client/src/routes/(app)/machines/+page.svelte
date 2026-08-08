<script lang="ts">
	import {
		Card,
		DonutChart,
		EmptyState,
		LineChart,
		Sparkline,
		StatCard,
		StatusDot,
		Tabs,
		chartColor,
		icons,
		type ChartSeries
	} from '@facile/muse';
	import {
		backend,
		type SessionStats,
		type SessionTimeline,
		type TokenInfo,
		type LiveSession
	} from '$lib/backend';
	import {
		bucketLabel,
		columnActive,
		columnTotals,
		formatDuration,
		formatTokens,
		hours,
		periodDelta
	} from '$lib/metrics';

	const ONLINE_WINDOW_MS = 11 * 60 * 1000;

	const RANGES = [
		{ id: '7d', label: '7 days' },
		{ id: '30d', label: '30 days' },
		{ id: 'all', label: 'All time' }
	];

	let tokens: TokenInfo[] = $state([]);
	/*
	 * `/api/tokens` is admin-only, so a plain user gets an error there while the sessions
	 * endpoints answer fine. The pairing list degrades; the charts must not.
	 */
	let tokensDenied = $state(false);
	let since = $state('7d');
	let timeline: SessionTimeline | null = $state(null);
	let stats: SessionStats | null = $state(null);
	let live: LiveSession[] = $state([]);

	const bucket = $derived(since === 'all' ? 'month' : 'day');
	const bucketUnit = $derived(bucket === 'month' ? 'month' : 'day');

	const machines = $derived(
		tokens.filter(
			(t) => t.scope !== 'admin' && t.scope !== 'user' && !t.name.startsWith('session')
		)
	);
	const online = $derived(machines.filter((m) => machineState(m) === 'connected').length);

	$effect(() => {
		const load = () =>
			backend
				.tokensList()
				.then((t) => {
					tokens = t ?? [];
					tokensDenied = false;
				})
				.catch(() => (tokensDenied = true));
		load();
		const id = setInterval(load, 30_000);
		return () => clearInterval(id);
	});

	$effect(() => {
		const load = () =>
			backend
				.sessionsLive()
				.then((l) => (live = l ?? []))
				.catch(() => {});
		load();
		const id = setInterval(load, 30_000);
		return () => clearInterval(id);
	});

	$effect(() => {
		backend
			.sessionsTimeline(since, bucket, 'machine')
			.then((t) => (timeline = t))
			.catch(() => (timeline = null));
	});

	$effect(() => {
		backend
			.sessionsStats(since, 'machine')
			.then((s) => (stats = s))
			.catch(() => (stats = null));
	});

	const labels = $derived.by(() => timeline?.labels ?? []);
	const tSeries = $derived.by(() => timeline?.series ?? []);

	const secondsPerBucket = $derived(columnTotals(tSeries.map((s) => s.seconds)));
	const sessionsPerBucket = $derived(columnTotals(tSeries.map((s) => s.sessions)));
	const tokensPerBucket = $derived(columnTotals(tSeries.map((s) => s.tokens_out)));
	const activePerBucket = $derived(columnActive(tSeries.map((s) => s.seconds)));

	const hoursSeries: ChartSeries[] = $derived(
		tSeries.map((s) => ({ name: s.key, data: s.seconds.map(hours) }))
	);

	const rows = $derived.by(() => stats?.rows ?? []);
	const totalSeconds = $derived(rows.reduce((total, r) => total + r.seconds, 0));
	const totalTokensOut = $derived(rows.reduce((total, r) => total + r.tokens_out, 0));
	const shareSlices = $derived(
		[...rows]
			.sort((a, b) => b.seconds - a.seconds)
			.slice(0, 6)
			.map((r) => ({ label: r.key, value: hours(r.seconds) }))
	);

	/* A machine can be working without holding an admin token we are allowed to list, so the
	   headline count falls back to whoever shows up in the session data. */
	const knownMachines = $derived.by(() => {
		if (machines.length > 0) return machines.length;
		const names = new Set<string>([...tSeries.map((s) => s.key), ...live.map((l) => l.machine)]);
		names.delete('Other');
		return names.size;
	});
	const workingNow = $derived(
		new Set(live.filter((l) => l.live && l.machine_online).map((l) => l.machine)).size
	);

	const timeDelta = $derived(periodDelta(secondsPerBucket, bucketUnit));
	const tokensDelta = $derived(periodDelta(tokensPerBucket, bucketUnit));
	const hasTimeline = $derived(labels.length > 0 && tSeries.length > 0);

	function ago(iso: string): string {
		if (!iso) return 'never';
		const then = new Date(iso).getTime();
		if (isNaN(then)) return 'never';
		const s = Math.floor((Date.now() - then) / 1000);
		if (s < 60) return `${s}s ago`;
		if (s < 3600) return `${Math.floor(s / 60)}m ago`;
		if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
		return `${Math.floor(s / 86400)}d ago`;
	}

	/* Three states, not a boolean: a machine that has never checked in needs a different fix
	   from one that checked in this morning and stopped. */
	function machineState(m: TokenInfo): 'connected' | 'idle' | 'never' {
		const then = new Date(m.last_seen).getTime();
		if (!m.last_seen || isNaN(then)) return 'never';
		return Date.now() - then < ONLINE_WINDOW_MS ? 'connected' : 'idle';
	}

	const tones = { connected: 'success', idle: 'neutral', never: 'warning' } as const;

	function statusLabel(m: TokenInfo): string {
		const s = machineState(m);
		if (s === 'connected') return 'connected';
		if (s === 'never') return 'never synced';
		return `idle · ${ago(m.last_seen)}`;
	}

	function machineSeconds(name: string): number {
		return rows.find((r) => r.key === name)?.seconds ?? 0;
	}

	function machineSpark(name: string): number[] {
		return tSeries.find((s) => s.key === name)?.seconds.map(hours) ?? [];
	}

	/* Same slot as the line chart's legend, so a machine keeps one colour across the page. */
	function machineColor(name: string): string {
		const i = tSeries.findIndex((s) => s.key === name);
		return i < 0 ? 'var(--color-fc-chart-1)' : chartColor(i);
	}
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-col gap-2">
		<h1 class="text-fc-2xl font-semibold text-fc-fg">Machines</h1>
		<p class="text-fc-sm text-fc-fg-muted">
			Machines and agents syncing with this brain. Connected means a sync in the last ten
			minutes.
		</p>
	</div>

	<section class="flex flex-col gap-4">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<h2 class="text-fc-lg font-semibold text-fc-fg">Fleet activity</h2>
			<Tabs items={RANGES} bind:value={since} label="Time range" />
		</div>

		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
			<StatCard label="Machines" value={knownMachines}>
				<Sparkline data={activePerBucket} class="mt-3" valueFormat={(n) => `${n}`} />
			</StatCard>
			<StatCard
				label="Working now"
				value={workingNow}
				delta={tokensDenied ? undefined : `${online} synced in the last 10 minutes`}
			>
				<Sparkline
					data={sessionsPerBucket}
					class="mt-3"
					color="var(--color-fc-chart-3)"
					valueFormat={(n) => `${n}`}
				/>
			</StatCard>
			<StatCard label="Active time" value={formatDuration(totalSeconds)} delta={timeDelta}>
				<Sparkline
					data={secondsPerBucket.map(hours)}
					class="mt-3"
					color="var(--color-fc-chart-2)"
					showLast
				/>
			</StatCard>
			<StatCard label="Tokens out" value={formatTokens(totalTokensOut)} delta={tokensDelta}>
				<Sparkline data={tokensPerBucket} class="mt-3" color="var(--color-fc-chart-5)" />
			</StatCard>
		</div>

		{#if hasTimeline}
			<div class="grid gap-4 lg:grid-cols-3">
				<Card class="flex flex-col gap-4 lg:col-span-2">
					<p class="text-fc-sm font-medium text-fc-fg">
						Active time per {bucketUnit}, by machine
					</p>
					<LineChart
						series={hoursSeries}
						{labels}
						area
						height={240}
						class="flex-1"
						yFormat={(n) => `${n} h`}
						xFormat={(l) => bucketLabel(l)}
					/>
				</Card>
				<Card class="flex flex-col gap-4">
					<p class="text-fc-sm font-medium text-fc-fg">Share of active time</p>
					<DonutChart
						data={shareSlices}
						centerLabel="Active"
						centerValue={formatDuration(totalSeconds)}
						valueFormat={(n) => `${n} h`}
						class="flex-1"
					/>
				</Card>
			</div>
		{:else}
			<Card class="text-fc-sm text-fc-fg-muted">
				No recorded activity in this range yet.
			</Card>
		{/if}
	</section>

	{#if machines.length === 0}
		<EmptyState icon={icons.server} title="No machines connected yet">
			<p class="text-fc-sm text-fc-fg-muted">
				{#if tokensDenied}
					Only an admin can list paired machines — the activity above still reflects the whole
					fleet.
				{:else}
					Run <code class="rounded-fc-xs bg-fc-surface px-1.5 py-0.5 font-fc-mono text-fc-xs"
						>mycelium login https://mycelium.facile.studio</code
					> on a machine to pair it.
				{/if}
			</p>
		</EmptyState>
	{:else}
		<section class="flex flex-col gap-4">
			<h2 class="text-fc-lg font-semibold text-fc-fg">Paired machines</h2>
			<div class="grid gap-4 sm:grid-cols-2">
				{#each machines as machine (machine.name)}
					<Card class="flex flex-col gap-4">
						<div class="flex items-start justify-between gap-3">
							<div class="flex min-w-0 items-center gap-2.5">
								<iconify-icon
									icon={icons.server}
									width="18"
									height="18"
									class="block shrink-0 text-fc-fg-muted"
								></iconify-icon>
								<span class="truncate font-fc-mono text-fc-sm font-medium text-fc-fg">
									{machine.name}
								</span>
							</div>
							<StatusDot
								tone={tones[machineState(machine)]}
								label={statusLabel(machine)}
								pulse={machineState(machine) === 'connected'}
								class="shrink-0"
							/>
						</div>
						{#if machineSpark(machine.name).length > 0}
							<Sparkline
							data={machineSpark(machine.name)}
							height={28}
							color={machineColor(machine.name)}
							valueFormat={(n) => `${n} h`}
						/>
						{/if}
						<dl class="flex flex-col gap-1 text-fc-xs text-fc-fg-muted">
							<div class="flex justify-between gap-3">
								<dt>Last sync</dt>
								<dd class="tabular-nums">{ago(machine.last_seen)}</dd>
							</div>
							<div class="flex justify-between gap-3">
								<dt>Active in range</dt>
								<dd class="tabular-nums">{formatDuration(machineSeconds(machine.name))}</dd>
							</div>
							<div class="flex justify-between gap-3">
								<dt>Paired</dt>
								<dd class="tabular-nums">{machine.created_at?.slice(0, 10) || '—'}</dd>
							</div>
						</dl>
					</Card>
				{/each}
			</div>
		</section>
	{/if}
</div>
