<script lang="ts">
	import { backend, type RucheStatus } from '$lib/backend';
	import { getContext } from 'svelte';

	const getStatus = getContext<() => RucheStatus | null>('status');
	const refreshStatus = getContext<() => Promise<void>>('refreshStatus');

	let status = $derived(getStatus());

	async function createCell() {
		const name = prompt('Cell name:');
		if (!name) return;
		await backend.createCell(name);
		await refreshStatus();
	}

	async function switchCell(name: string) {
		await backend.useCell(name);
		await refreshStatus();
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-semibold">Cells</h2>
			<p class="text-sm text-muted">Switch between agent profiles.</p>
		</div>
		<button onclick={createCell} class="rounded-lg bg-primary px-3 py-1.5 text-sm font-medium text-primary-fg">
			New cell
		</button>
	</div>

	{#if status?.cells}
		<div class="space-y-2">
			{#each status.cells as cell}
				{@const active = cell.name === status?.active_cell}
				<div
					class="flex items-center justify-between rounded-lg border px-4 py-3 {active
						? 'border-primary bg-primary/5'
						: 'border-border bg-surface'}"
				>
					<div>
						<p class="text-sm font-medium">{cell.name}</p>
						<p class="text-xs text-muted">{cell.path}</p>
					</div>
					{#if active}
						<span class="rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">Active</span>
					{:else}
						<button
							onclick={() => switchCell(cell.name)}
							class="rounded-lg border border-border px-3 py-1 text-xs hover:bg-bg"
						>
							Use
						</button>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
