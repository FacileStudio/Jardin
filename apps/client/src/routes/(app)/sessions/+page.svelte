<script lang="ts">
	import { EmptyState, Tabs, icons } from '@facile/muse';
	import {
		backend,
		type SessionBlock,
		type SessionStats,
		type SessionTimeline,
		type UsageHistory,
		type UsageSnapshot
	} from '$lib/backend';
	import UsageMeter from '$lib/components/UsageMeter.svelte';
	import SessionsClaims from '$lib/components/SessionsClaims.svelte';
	import SessionsHistoryTab from '$lib/components/SessionsHistoryTab.svelte';
	import SessionsLive from '$lib/components/SessionsLive.svelte';
	import SessionsOverviewTab from '$lib/components/SessionsOverviewTab.svelte';

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
	const PAGE_TABS = [
		{ id: 'overview', label: 'Overview' },
		{ id: 'history', label: 'History' },
		{ id: 'usage', label: 'Usage' }
	];

	let tab = $state('overview');
	let since = $state('7d');
	let by = $state('project');
	let stats: SessionStats | null = $state(null);
	let recent: SessionBlock[] = $state([]);
	let timeline: SessionTimeline | null = $state(null);
	let usage: UsageSnapshot[] = $state([]);
	let usageLog: UsageHistory | null = $state(null);

	/* All time over daily buckets is a thousand pixels of noise; months keep the axis honest. */
	const bucket = $derived(since === 'all' ? 'month' : 'day');
	const bucketUnit = $derived(bucket === 'month' ? 'month' : 'day');

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
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-col gap-2">
		<h1 class="text-fc-2xl font-semibold text-fc-fg">Sessions</h1>
		<p class="text-fc-sm text-fc-fg-muted">
			Agent work sessions recorded across every machine that syncs with this brain.
		</p>
	</div>

	<SessionsLive />

	<SessionsClaims />

	<section class="flex flex-col gap-4">
		<div class="flex flex-col gap-1">
			<h2 class="text-fc-lg font-semibold text-fc-fg">Totals</h2>
			<p class="text-fc-sm text-fc-fg-muted">
				Sealed sessions only — whatever is running now lands here when it ends.
			</p>
		</div>

		<Tabs items={PAGE_TABS} bind:value={tab} />

		{#if rows.length === 0}
			<EmptyState
				icon={icons.history}
				title="No sessions recorded yet"
				description="Machines record agent activity automatically once they run mycelium v0.5 or later."
			/>
		{:else if tab === 'overview'}
			<div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
				<Tabs items={RANGES} bind:value={since} label="Time range" />
				<Tabs items={GROUPS} bind:value={by} label="Group sessions by" />
			</div>

			<SessionsOverviewTab {rows} {timeline} {by} {bucketUnit} />
		{:else if tab === 'history'}
			<div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
				<Tabs items={RANGES} bind:value={since} label="Time range" />
				<Tabs items={GROUPS} bind:value={by} label="Group sessions by" />
			</div>

			<SessionsHistoryTab {rows} {recent} {by} groups={GROUPS} />
		{:else if tab === 'usage'}
			<h2 class="text-fc-lg font-semibold text-fc-fg">Subscription windows</h2>
			<p class="text-fc-sm text-fc-fg-muted">
				Usage against subscription limits recorded by each agent.
			</p>
			<UsageMeter snapshots={usage} history={usageLog} />
		{/if}
	</section>
</div>
