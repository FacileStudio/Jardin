<script lang="ts">
	import {
		BarChart,
		Card,
		DonutChart,
		LineChart,
		Sparkline,
		StatCard,
		type ChartSeries
	} from '@facile/muse';
	import type { SessionStatRow, SessionTimeline } from '$lib/backend';
	import {
		bucketLabel,
		columnTotals,
		formatCost,
		formatDuration,
		formatTokens,
		hours,
		periodDelta
	} from '$lib/metrics';

	let {
		rows,
		timeline,
		by,
		bucketUnit
	}: {
		rows: SessionStatRow[];
		timeline: SessionTimeline | null;
		by: string;
		bucketUnit: string;
	} = $props();

	/* Eight bars is where a horizontal chart stops being readable in a card; the table below
	   still lists every row, so nothing is hidden. */
	const CHART_ROWS = 8;

	const totalSeconds = $derived(rows.reduce((sum, r) => sum + r.seconds, 0));
	const totalSessions = $derived(rows.reduce((sum, r) => sum + r.sessions, 0));
	const totalTokensOut = $derived(rows.reduce((sum, r) => sum + r.tokens_out, 0));

	const totalCacheRead = $derived(rows.reduce((sum, r) => sum + r.cache_read, 0));
	const totalTokensIn = $derived(rows.reduce((sum, r) => sum + r.tokens_in, 0));
	const totalCost = $derived(rows.reduce((sum, r) => sum + r.cost_total, 0));

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
	const tokensInPerBucket = $derived(columnTotals(tSeries.map((s) => s.tokens_in)));
	const costPerBucket = $derived(columnTotals(tSeries.map((s) => s.cost_total)));
	const hasTimeline = $derived(labels.length > 0 && tSeries.length > 0);

	const timeDelta = $derived(periodDelta(secondsPerBucket, bucketUnit));
	const sessionsDelta = $derived(periodDelta(sessionsPerBucket, bucketUnit));
	const tokensDelta = $derived(periodDelta(tokensPerBucket, bucketUnit));
	const cacheDelta = $derived(periodDelta(cachePerBucket, bucketUnit));
	const tokensInDelta = $derived(periodDelta(tokensInPerBucket, bucketUnit));
	const costDelta = $derived(periodDelta(costPerBucket, bucketUnit));
</script>

<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 [&>*]:min-w-0">
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
	<StatCard label="Cache read" value={formatTokens(totalCacheRead)} delta={cacheDelta}>
		<Sparkline data={cachePerBucket} class="mt-3" color="var(--color-fc-chart-5)" />
	</StatCard>
	<StatCard label="Tokens out" value={formatTokens(totalTokensOut)} delta={tokensDelta}>
		<Sparkline data={tokensPerBucket} class="mt-3" color="var(--color-fc-chart-2)" />
	</StatCard>
	<StatCard label="Tokens in" value={formatTokens(totalTokensIn)} delta={tokensInDelta}>
		<Sparkline data={tokensInPerBucket} class="mt-3" color="var(--color-fc-chart-4)" />
	</StatCard>
	<StatCard label="Est. cost" value={formatCost(totalCost)} delta={costDelta}>
		<Sparkline
			data={costPerBucket}
			class="mt-3"
			color="var(--color-fc-chart-1)"
			valueFormat={(n) => formatCost(n)}
		/>
	</StatCard>
</div>

{#if hasTimeline}
	<div class="grid gap-4 lg:grid-cols-3 [&>*]:min-w-0">
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
