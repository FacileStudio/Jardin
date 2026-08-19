<script lang="ts">
	import { Table } from '@facile/muse';
	import type { SessionBlock, SessionStatRow } from '$lib/backend';
	import { formatCost, formatDuration, formatTokens } from '$lib/metrics';

	let {
		rows,
		recent,
		by,
		groups
	}: {
		rows: SessionStatRow[];
		recent: SessionBlock[];
		by: string;
		groups: { id: string; label: string }[];
	} = $props();

	function formatEnded(iso: string): string {
		const d = new Date(iso);
		const month = d.toLocaleString('en-US', { month: 'short' });
		const day = String(d.getDate()).padStart(2, '0');
		const hh = String(d.getHours()).padStart(2, '0');
		const mm = String(d.getMinutes()).padStart(2, '0');
		return `${month} ${day} ${hh}:${mm}`;
	}

	function blockDuration(b: SessionBlock): string {
		const seconds = Math.max(
			0,
			(new Date(b.ended_at).getTime() - new Date(b.started_at).getTime()) / 1000
		);
		return formatDuration(seconds);
	}
</script>

<Table>
	<thead>
		<tr>
			<th scope="col">{groups.find((g) => g.id === by)?.label ?? by}</th>
			<th scope="col" class="text-right">Sessions</th>
			<th scope="col" class="text-right">Active</th>
			<th scope="col" class="text-right">Tokens in</th>
			<th scope="col" class="text-right">Tokens out</th>
			<th scope="col" class="text-right">Est. cost</th>
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
				<td class="text-right tabular-nums">{formatCost(row.cost_total)}</td>
			</tr>
		{/each}
	</tbody>
</Table>

{#if recent.length > 0}
	<Table>
		<thead>
			<tr>
				<th scope="col">Ended</th>
				<th scope="col">Project</th>
				<th scope="col">Machine / agent</th>
				<th scope="col">Model</th>
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
					<td class="whitespace-nowrap font-fc-mono text-fc-xs text-fc-fg-muted">
						{block.model ?? '—'}
					</td>
					<td class="text-right tabular-nums">{blockDuration(block)}</td>
					<td class="text-right tabular-nums">{formatTokens(block.tokens_out)}</td>
				</tr>
			{/each}
		</tbody>
	</Table>
{/if}
