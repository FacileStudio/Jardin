<script lang="ts">
	import { Card, LineChart, icons, type ChartSeries } from '@facile/muse';
	import type { UsageHistory, UsageSnapshot, UsageWindow } from '$lib/backend';
	import { formatAge, formatCountdown, formatSpan } from '$lib/metrics';

	let {
		snapshots = [],
		history = null
	}: {
		snapshots?: UsageSnapshot[];
		history?: UsageHistory | null;
	} = $props();

	/*
	 * muse has no meter, and a meter is not a chart: it shows one value against a known
	 * ceiling, so it gets bars built from divs rather than an axis. Pressure is banded
	 * because "68%" of a five-hour window matters differently from 92% — the bands are
	 * where a human changes behaviour, not decoration.
	 */
	const CALM = 60;
	const HOT = 85;

	function tint(pct: number): string {
		if (pct >= HOT) return 'var(--color-fc-chart-4)';
		if (pct >= CALM) return 'var(--color-fc-chart-2)';
		return 'var(--color-fc-chart-5)';
	}

	function clamp(pct: number): number {
		return Math.max(0, Math.min(100, Number.isFinite(pct) ? pct : 0));
	}

	function pctText(pct: number): string {
		return `${pct.toFixed(pct < 10 ? 1 : 0)}%`;
	}

	/*
	 * Once the window has rolled over the recorded percentage is history, so it loses the
	 * pressure tint: a red 90% bar for a window that already reset raises an alarm about a
	 * situation that no longer exists.
	 */
	function fillColor(limit: UsageWindow, pct: number): string | undefined {
		return limit.expired ? undefined : tint(pct);
	}

	/*
	 * The server computes the countdown against the clock the reset was recorded on, so its
	 * value wins; `resets_at` is only the fallback for a payload that predates the field.
	 */
	function resetText(limit: UsageWindow, pct: number): string {
		if (limit.expired) return `${pctText(pct)} when last seen · window has since reset`;
		if (limit.resets_in_seconds !== undefined && limit.resets_in_seconds !== null) {
			return `resets in ${formatSpan(limit.resets_in_seconds)}`;
		}
		if (limit.resets_at) return `resets in ${formatCountdown(limit.resets_at)}`;
		return 'reset time unknown';
	}

	function meterLabel(limit: UsageWindow, pct: number): string {
		const name = limit.label || limit.key;
		if (limit.expired) {
			return `${name}: ${pctText(pct)} when last seen, window has since reset`;
		}
		return `${name} used`;
	}

	function snapshotAge(snapshot: UsageSnapshot): string {
		if (snapshot.age_seconds !== undefined && snapshot.age_seconds !== null) {
			return snapshot.age_seconds < 60 ? 'just now' : `${formatSpan(snapshot.age_seconds)} ago`;
		}
		return formatAge(snapshot.updated_at);
	}

	const historySeries: ChartSeries[] = $derived(
		(history?.series ?? []).slice(0, 6).map((s) => ({
			name: s.label || s.key,
			/* muse skips non-finite points, so a missing sample becomes a gap in the line
			   instead of a dive to zero it never took. */
			data: s.values.map((v) => (v === null || v === undefined ? NaN : v))
		}))
	);
	const hasHistory = $derived((history?.labels?.length ?? 0) > 1 && historySeries.length > 0);

	function clockLabel(iso: string): string {
		const d = new Date(iso);
		if (isNaN(d.getTime())) return iso;
		return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
	}
</script>

<div class="flex flex-col gap-4">
	{#if snapshots.length === 0}
		<Card class="flex flex-col gap-2">
			<p class="text-fc-sm font-medium text-fc-fg">Plan usage</p>
			<p class="text-fc-sm text-fc-fg-muted">
				Nothing recorded yet. A machine reports its subscription windows once <code
					class="rounded-fc-xs bg-fc-surface px-1.5 py-0.5 font-fc-mono text-fc-xs"
					>jardin install claude</code
				> has wired the status line and an agent session has made its first request.
			</p>
		</Card>
	{:else}
		<div class="grid gap-4 {snapshots.length > 1 ? 'lg:grid-cols-2' : ''}">
			{#each snapshots as snapshot (snapshot.machine)}
				<Card class="flex flex-col gap-4">
					<div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
						<span class="flex min-w-0 items-center gap-2">
							<iconify-icon
								icon={icons.server}
								width="16"
								height="16"
								class="block shrink-0 text-fc-fg-muted"
							></iconify-icon>
							<span class="truncate font-fc-mono text-fc-sm font-medium text-fc-fg">
								{snapshot.machine}
							</span>
						</span>
						<span class="text-fc-xs text-fc-fg-muted">
							{snapshot.model ? `${snapshot.model} · ` : ''}{snapshotAge(snapshot)}
						</span>
					</div>

					{#if snapshot.stale}
						<!-- A gap is the normal shape of this data, not a fault: no warning colour. -->
						<p class="text-fc-xs text-fc-fg-muted">
							Last reported {snapshotAge(snapshot)}. The status line only reports while Claude Code
							is running, so a gap means nobody reported — not that nothing was used.
						</p>
					{/if}

					{#if snapshot.windows.length === 0}
						<p class="text-fc-sm text-fc-fg-muted">No window reported.</p>
					{:else}
						<div class="flex flex-col gap-3">
							{#each snapshot.windows as limit (limit.key)}
								{@const pct = clamp(limit.used_percentage)}
								<div class="flex flex-col gap-1.5">
									<div class="flex items-baseline justify-between gap-3">
										<span
											class="truncate text-fc-sm {limit.expired ? 'text-fc-fg-muted' : 'text-fc-fg'}"
										>
											{limit.label || limit.key}
										</span>
										<span
											class="shrink-0 text-fc-sm font-medium tabular-nums {limit.expired
												? 'text-fc-fg-muted'
												: 'text-fc-fg'}"
										>
											{pctText(pct)}
										</span>
									</div>
									<div
										class="h-2 w-full overflow-hidden rounded-fc-full bg-fc-surface"
										role="meter"
										aria-valuenow={Math.round(pct)}
										aria-valuemin={0}
										aria-valuemax={100}
										aria-label={meterLabel(limit, pct)}
									>
										<div
											class="h-full rounded-fc-full transition-[width] duration-500 {limit.expired
												? 'bg-fc-fg-muted/40'
												: ''}"
											style:width="{pct}%"
											style:background-color={fillColor(limit, pct)}
										></div>
									</div>
									<p class="text-fc-xs text-fc-fg-muted">{resetText(limit, pct)}</p>
								</div>
							{/each}
						</div>
					{/if}
				</Card>
			{/each}
		</div>

		{#if hasHistory}
			<Card class="flex flex-col gap-4">
				<p class="text-fc-sm font-medium text-fc-fg">Burn-down</p>
				<LineChart
					series={historySeries}
					labels={history?.labels ?? []}
					area
					height={200}
					yFormat={(n) => `${Math.round(n)}%`}
					xFormat={(label) => clockLabel(label)}
				/>
			</Card>
		{/if}
	{/if}
</div>
