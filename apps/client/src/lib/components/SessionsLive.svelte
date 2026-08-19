<script lang="ts">
	import { Card, StatusDot, icons } from '@facile/muse';
	import { backend, type LiveSession } from '$lib/backend';
	import { formatDuration, formatTokens } from '$lib/metrics';

	let live: LiveSession[] = $state([]);

	$effect(() => {
		const load = () =>
			backend
				.sessionsLive()
				.then((l) => (live = l ?? []))
				.catch(() => {});
		load();
		const timer = setInterval(load, 30_000);
		return () => clearInterval(timer);
	});

	function elapsed(iso: string): string {
		return formatDuration(Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000));
	}

	/* Three states, not a boolean: a machine that went offline, an agent thinking, and an
	   agent that has been idle for twenty minutes all need different reactions. */
	function liveState(s: LiveSession): 'active' | 'idle' | 'offline' {
		if (!s.machine_online) return 'offline';
		return s.live ? 'active' : 'idle';
	}

	const liveTones = { active: 'success', idle: 'warning', offline: 'neutral' } as const;

	function liveLabel(s: LiveSession): string {
		const state = liveState(s);
		if (state === 'offline') return 'machine offline';
		if (state === 'active') return 'active';
		return `idle ${Math.max(1, Math.round(s.idle_seconds / 60))}m`;
	}
</script>

<section class="flex flex-col gap-4">
	<div class="flex flex-wrap items-start justify-between gap-3">
		<div class="flex min-w-0 flex-col gap-1">
			<h2 class="text-fc-lg font-semibold text-fc-fg">Running now</h2>
			<p class="text-fc-sm text-fc-fg-muted">Refreshed every thirty seconds.</p>
		</div>
		<!-- The phone's nav bar only holds four destinations, so this is how Machines is
		     reached from a phone. -->
		<a
			href="/machines"
			class="inline-flex shrink-0 items-center gap-1 text-fc-sm text-fc-fg-muted transition-colors hover:text-fc-fg focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-fc-ring"
		>
			<iconify-icon icon={icons.server} width="16" height="16" class="block"></iconify-icon>
			Machines
		</a>
	</div>

	{#if live.length === 0}
		<Card class="text-fc-sm text-fc-fg-muted">No session is running.</Card>
	{:else}
		<div class="flex flex-col gap-2">
			{#each live as session (session.machine + session.project + session.started_at)}
				<Card
					class="flex flex-wrap items-center gap-x-4 gap-y-2 py-3 {session.machine_online
						? ''
						: 'opacity-60'}"
				>
					<StatusDot
						tone={liveTones[liveState(session)]}
						label={liveLabel(session)}
						pulse={liveState(session) === 'active'}
					/>
					<span class="text-fc-sm font-medium text-fc-fg">{session.project}</span>
					<span class="font-fc-mono text-fc-xs text-fc-fg-muted">
						{session.machine}/{session.agent}
					</span>
					{#if session.branch}
						<span
							class="rounded-fc-xs bg-fc-surface px-1.5 py-0.5 font-fc-mono text-fc-xs text-fc-fg-muted"
						>
							{session.branch}
						</span>
					{/if}
					<span class="ml-auto text-fc-xs tabular-nums text-fc-fg-muted">
						{elapsed(session.started_at)} · {formatTokens(session.tokens_out)} out
					</span>
				</Card>
			{/each}
		</div>
	{/if}
</section>
