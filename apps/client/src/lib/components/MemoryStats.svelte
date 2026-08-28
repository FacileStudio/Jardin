<script lang="ts">
	import { Card, DonutChart, LineChart, Sparkline, StatCard } from '@facile/muse';
	import type { FileEntry } from '$lib/backend';
	import {
		bucketByDay,
		bucketLabel,
		dayKey,
		dayWindow,
		formatAge,
		formatBytes,
		periodDelta,
		sum
	} from '$lib/metrics';

	let {
		files,
		grouped
	}: {
		files: FileEntry[];
		grouped: [string, FileEntry[]][];
	} = $props();

	/*
	 * Thirty days of history: long enough that a habit shows up, short enough that a wiki
	 * touched twice last year does not squash the recent weeks into one flat pixel.
	 */
	const WINDOW_DAYS = 30;

	/*
	 * Every number below comes out of the tree the page already loaded — `size` and `mod_time`
	 * ride along with each entry, so there is nothing to ask the server for.
	 */
	const dayLabels = $derived.by(() => dayWindow(WINDOW_DAYS));
	const dailyPages = $derived(
		bucketByDay(
			(files ?? []).map((f) => ({ iso: f.mod_time })),
			dayLabels
		)
	);
	const dailyBytes = $derived(
		bucketByDay(
			(files ?? []).map((f) => ({ iso: f.mod_time, weight: f.size })),
			dayLabels
		)
	);
	const dailyFolders = $derived.by(() => {
		const seen = dayLabels.map(() => new Set<string>());
		const index = new Map(dayLabels.map((l, i) => [l, i]));
		for (const f of files ?? []) {
			const d = new Date(f.mod_time);
			if (isNaN(d.getTime())) continue;
			const i = index.get(dayKey(d));
			if (i === undefined) continue;
			const parts = f.path.split('/');
			seen[i].add(parts.length > 2 ? parts[1] : '/');
		}
		return seen.map((s) => s.size);
	});

	const totalSize = $derived(sum((files ?? []).map((f) => f.size)));
	const newest = $derived.by(() =>
		(files ?? []).reduce<FileEntry | null>(
			(best, f) =>
				!best || new Date(f.mod_time).getTime() > new Date(best.mod_time).getTime() ? f : best,
			null
		)
	);

	const folderSlices = $derived(
		(grouped ?? []).map(([folder, entries]) => ({
			label: folder === '/' ? 'root' : folder,
			value: (entries ?? []).length
		}))
	);
	const folderCounts = $derived(folderSlices.map((s) => s.value));

	const pagesDelta = $derived(periodDelta(dailyPages, 'day'));
	const sizeDelta = $derived(periodDelta(dailyBytes, 'day'));

	const activitySeries = $derived([{ name: 'Pages touched', data: dailyPages }]);

	function label(path: string) {
		return path.split('/').pop()!.replace(/\.md$/, '');
	}
</script>

<section class="flex flex-col gap-4">
	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4 [&>*]:min-w-0">
		<StatCard label="Pages" value={files.length} delta={pagesDelta}>
			<Sparkline data={dailyPages} class="mt-3" showLast />
		</StatCard>
		<StatCard label="Folders" value={folderSlices.length}>
			<Sparkline
				data={dailyFolders}
				class="mt-3"
				color="var(--color-fc-chart-3)"
				valueFormat={(n) => `${n}`}
			/>
		</StatCard>
		<StatCard label="Size" value={formatBytes(totalSize)} delta={sizeDelta}>
			<Sparkline data={dailyBytes} class="mt-3" color="var(--color-fc-chart-2)" />
		</StatCard>
		<StatCard
			label="Last written"
			value={newest ? formatAge(newest.mod_time) : '—'}
			delta={newest ? label(newest.path) : undefined}
		>
			<Sparkline data={folderCounts} class="mt-3" color="var(--color-fc-chart-5)" />
		</StatCard>
	</div>

	<div class="grid gap-4 lg:grid-cols-3 [&>*]:min-w-0">
		<Card class="flex flex-col gap-4 lg:col-span-2">
			<p class="text-fc-sm font-medium text-fc-fg">
				Pages written per day · last {WINDOW_DAYS} days
			</p>
			<LineChart
				series={activitySeries}
				labels={dayLabels}
				area
				height={240}
				class="flex-1"
				yFormat={(n) => `${n}`}
				xFormat={(l) => bucketLabel(l)}
			/>
		</Card>
		<Card class="flex flex-col gap-4">
			<p class="text-fc-sm font-medium text-fc-fg">Pages per folder</p>
			<DonutChart
				data={folderSlices}
				centerLabel="Pages"
				centerValue={files.length}
				class="flex-1"
			/>
		</Card>
	</div>
</section>
