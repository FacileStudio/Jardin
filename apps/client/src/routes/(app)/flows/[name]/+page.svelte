<script lang="ts">
	import { page } from '$app/state';
	import { Alert, Badge, Card, icons, toast } from '@facile/muse';
	import { backend, type FlowDetail } from '$lib/backend';
	import CodeView from '$lib/components/CodeView.svelte';

	const name = $derived(page.params.name ?? '');
	let detail: FlowDetail | null = $state(null);
	let loading = $state(true);

	$effect(() => {
		const n = name;
		loading = true;
		backend
			.flowGet(n)
			.then((d) => (detail = d))
			.catch((e) => {
				detail = null;
				toast.danger(e instanceof Error ? e.message : `Could not load flow "${n}".`);
			})
			.finally(() => (loading = false));
	});
</script>

<CodeView
	title={name}
	path="flows/{name}.yml"
	icon={icons.plug}
	backHref="/flows"
	backLabel="Flows"
	content={detail?.raw ?? ''}
	{loading}
>
	{#if detail?.parse_error}
		<Alert tone="warning" title="This flow does not parse">
			{detail.parse_error}
		</Alert>
	{:else if detail?.summary}
		<Card class="flex flex-col gap-3">
			{#if detail.summary.description}
				<p class="text-fc-sm text-fc-fg-muted">{detail.summary.description}</p>
			{/if}
			<div class="flex flex-col gap-2">
				{#each detail.summary.steps as step (step.name)}
					<div class="flex flex-wrap items-center gap-2 rounded-fc-sm bg-fc-surface px-3 py-2">
						<span class="font-fc-mono text-fc-sm text-fc-fg">{step.name}</span>
						<Badge tone={step.kind === 'type' ? 'info' : 'neutral'}>
							{step.kind === 'type' ? step.type : 'shell'}
						</Badge>
						{#if step.depends_on?.length}
							<span class="text-fc-xs text-fc-fg-muted">
								after {step.depends_on.join(', ')}
							</span>
						{/if}
						{#if step.needs && Object.keys(step.needs).length > 0}
							<span class="text-fc-xs text-fc-fg-muted">
								needs {Object.keys(step.needs).join(', ')}
							</span>
						{/if}
					</div>
				{/each}
			</div>
		</Card>
	{/if}
</CodeView>
