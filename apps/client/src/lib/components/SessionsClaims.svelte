<script lang="ts">
	import { Button, Card, StatusDot, icons } from '@facile/muse';
	import { backend, type Claim } from '$lib/backend';
	import { formatDuration } from '$lib/metrics';

	let claims: Claim[] = $state([]);
	let releasing: string | null = $state(null);

	function loadClaims() {
		return backend
			.claimsList()
			.then((c) => (claims = c ?? []))
			.catch(() => {});
	}

	$effect(() => {
		loadClaims();
		const timer = setInterval(loadClaims, 30_000);
		return () => clearInterval(timer);
	});

	function claimKey(c: Claim): string {
		return `${c.project}/${c.machine}/${c.agent}`;
	}

	async function releaseClaim(c: Claim) {
		const key = claimKey(c);
		releasing = key;
		try {
			await backend.claimRelease(c.project, c.machine, c.agent);
			await loadClaims();
		} catch {
		} finally {
			releasing = null;
		}
	}

	function elapsed(iso: string): string {
		return formatDuration(Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000));
	}

	const liveTones = { active: 'success', idle: 'warning', offline: 'neutral' } as const;

	function claimState(c: Claim): 'active' | 'idle' | 'offline' {
		if (!c.machine_online) return 'offline';
		return c.live ? 'active' : 'idle';
	}

	function claimLabel(c: Claim): string {
		const state = claimState(c);
		if (state === 'offline') return 'machine offline';
		if (state === 'active') return 'active';
		return 'idle';
	}

	function claimBodyPreview(body?: string): string {
		if (!body) return '';
		return body.split('\n').slice(0, 3).join('\n');
	}
</script>

<section class="flex flex-col gap-4">
	<div class="flex flex-col gap-1">
		<h2 class="text-fc-lg font-semibold text-fc-fg">Active claims</h2>
		<p class="text-fc-sm text-fc-fg-muted">
			In-flight task leases across every machine — release one to let another agent take over.
		</p>
	</div>

	{#if claims.length === 0}
		<Card class="text-fc-sm text-fc-fg-muted">No active claim.</Card>
	{:else}
		<div class="flex flex-col gap-2">
			{#each claims as claim (claimKey(claim))}
				<Card
					class="flex flex-col gap-2 py-3 {claim.machine_online ? '' : 'opacity-60'}"
				>
					<div class="flex flex-wrap items-center gap-x-4 gap-y-2">
						<StatusDot
							tone={liveTones[claimState(claim)]}
							label={claimLabel(claim)}
							pulse={claimState(claim) === 'active'}
						/>
						<span class="text-fc-sm font-medium text-fc-fg">{claim.project}</span>
						<span class="font-fc-mono text-fc-xs text-fc-fg-muted">
							{claim.machine}/{claim.agent}
						</span>
						{#if claim.branch}
							<span
								class="rounded-fc-xs bg-fc-surface px-1.5 py-0.5 font-fc-mono text-fc-xs text-fc-fg-muted"
							>
								{claim.branch}
							</span>
						{/if}
						<span class="text-fc-xs tabular-nums text-fc-fg-muted">
							since {elapsed(claim.started_at)}
						</span>
						<Button
							class="ml-auto"
							variant="ghost-danger"
							size="sm"
							icon={icons.close}
							aria-label="Release claim on {claim.project} by {claim.machine}/{claim.agent}"
							disabled={releasing === claimKey(claim)}
							onclick={() => releaseClaim(claim)}
						>
							{releasing === claimKey(claim) ? 'Releasing…' : 'Release'}
						</Button>
					</div>
					{#if claim.task}
						<p class="text-fc-sm text-fc-fg">{claim.task}</p>
					{/if}
					{#if claim.body}
						<pre
							class="whitespace-pre-wrap rounded-fc-xs bg-fc-surface px-2 py-1.5 font-fc-mono text-fc-xs text-fc-fg-muted">{claimBodyPreview(
								claim.body
							)}</pre>
					{/if}
				</Card>
			{/each}
		</div>
	{/if}
</section>
