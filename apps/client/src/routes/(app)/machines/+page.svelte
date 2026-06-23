<script lang="ts">
	import Icon from '@iconify/svelte';
	import { backend, type TokenInfo } from '$lib/backend';

	let tokens: TokenInfo[] = $state([]);
	let machines = $derived(tokens.filter((t) => t.name !== 'session'));

	$effect(() => {
		const load = () => backend.tokensList().then((t) => (tokens = t)).catch(() => {});
		load();
		const id = setInterval(load, 30000);
		return () => clearInterval(id);
	});

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

	function online(iso: string): boolean {
		if (!iso) return false;
		const then = new Date(iso).getTime();
		return !isNaN(then) && Date.now() - then < 11 * 60 * 1000;
	}
</script>

<div class="space-y-6">
	<div>
		<h2 class="text-2xl font-semibold tracking-tight">Machines</h2>
		<p class="text-sm text-muted-foreground">Machines and agents syncing with this brain. Connected = synced in the last ~10 min.</p>
	</div>

	{#if machines.length === 0}
		<div class="rounded-lg border border-dashed border-border p-8 text-center">
			<p class="text-sm text-muted-foreground">
				No machines connected yet. Run <code class="rounded bg-accent px-1 py-0.5 text-xs">ruche login https://ruche.facile.studio</code> on a machine.
			</p>
		</div>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2">
			{#each machines as m}
				<div class="rounded-lg border border-border p-4">
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-2">
							<Icon icon={m.name === 'session' ? 'solar:laptop-linear' : 'solar:server-square-linear'} class="size-5 text-muted-foreground" />
							<span class="font-medium">{m.name}</span>
						</div>
						<span class="flex items-center gap-1.5 text-xs {online(m.last_seen) ? 'text-green-600' : 'text-muted-foreground'}">
							<span class="size-2 rounded-full {online(m.last_seen) ? 'bg-green-500' : 'bg-muted-foreground/40'}"></span>
							{online(m.last_seen) ? 'connected' : 'idle'}
						</span>
					</div>
					<dl class="mt-3 space-y-1 text-xs text-muted-foreground">
						<div class="flex justify-between"><dt>Last sync</dt><dd>{ago(m.last_seen)}</dd></div>
						<div class="flex justify-between"><dt>Added</dt><dd>{m.created_at?.slice(0, 10) || '—'}</dd></div>
					</dl>
				</div>
			{/each}
		</div>
	{/if}
</div>
