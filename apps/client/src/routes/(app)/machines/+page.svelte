<script lang="ts">
	import { Card, StatCard, StatusDot, icons } from '@facile/muse';
	import { backend, type TokenInfo } from '$lib/backend';
	import EmptyState from '$lib/components/EmptyState.svelte';

	const ONLINE_WINDOW_MS = 11 * 60 * 1000;

	let tokens: TokenInfo[] = $state([]);

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
				.then((t) => (tokens = t))
				.catch(() => {});
		load();
		const id = setInterval(load, 30_000);
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
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-col gap-2">
		<h1 class="text-fc-2xl font-semibold text-fc-fg">Machines</h1>
		<p class="text-fc-sm text-fc-fg-muted">
			Machines and agents syncing with this brain. Connected means a sync in the last ten
			minutes.
		</p>
	</div>

	{#if machines.length === 0}
		<EmptyState icon={icons.server} title="No machines connected yet">
			<p class="text-fc-sm text-fc-fg-muted">
				Run <code class="rounded-fc-xs bg-fc-surface px-1.5 py-0.5 font-fc-mono text-fc-xs"
					>mycelium login https://mycelium.facile.studio</code
				> on a machine to pair it.
			</p>
		</EmptyState>
	{:else}
		<section class="flex flex-col gap-4">
			<div class="grid gap-4 sm:grid-cols-2">
				<StatCard label="Machines" value={machines.length} />
				<StatCard label="Connected now" value={online} delta="synced in the last 10 minutes" />
			</div>

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
						<dl class="flex flex-col gap-1 text-fc-xs text-fc-fg-muted">
							<div class="flex justify-between gap-3">
								<dt>Last sync</dt>
								<dd class="tabular-nums">{ago(machine.last_seen)}</dd>
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
