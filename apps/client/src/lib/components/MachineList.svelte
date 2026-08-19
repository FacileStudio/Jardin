<script lang="ts">
	import { Card, Sparkline, StatusDot, chartColor, icons } from '@facile/muse';
	import type { SessionStatRow, TimelineSeries, TokenInfo } from '$lib/backend';
	import { formatDuration, hours } from '$lib/metrics';

	let {
		machines,
		rows,
		tSeries
	}: {
		machines: TokenInfo[];
		rows: SessionStatRow[];
		tSeries: TimelineSeries[];
	} = $props();

	const ONLINE_WINDOW_MS = 11 * 60 * 1000;

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

<section class="flex flex-col gap-4">
	<h2 class="text-fc-lg font-semibold text-fc-fg">Paired machines</h2>
	<div class="grid gap-4 sm:grid-cols-2 [&>*]:min-w-0">
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
