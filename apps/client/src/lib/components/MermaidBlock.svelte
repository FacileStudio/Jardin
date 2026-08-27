<script lang="ts">
	import { Button, Card, Spinner, icons, toast } from '@facile/muse';
	import mermaid from 'mermaid';

	let { code = '' }: { code: string } = $props();

	let svgHtml = $state('');
	let error = $state<string | null>(null);
	let loading = $state(true);
	let showSource = $state(false);

	async function renderDiagram(diagramCode: string) {
		const trimmed = diagramCode.trim();
		if (!trimmed) {
			svgHtml = '';
			loading = false;
			return;
		}

		loading = true;
		error = null;

		try {
			mermaid.initialize({
				startOnLoad: false,
				theme: 'dark',
				fontFamily: 'inherit',
				securityLevel: 'loose'
			});
			const id = `mermaid-svg-${Math.random().toString(36).slice(2, 9)}`;
			const result = await mermaid.render(id, trimmed);
			svgHtml = result.svg;
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : String(e);
			error = msg;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		renderDiagram(code);
	});

	function copyToClipboard(text: string) {
		navigator.clipboard.writeText(text);
		toast.success('Code copied to clipboard');
	}
</script>

<Card class="my-4 overflow-hidden p-0">
	<div
		class="flex items-center justify-between border-b border-fc-border bg-fc-surface-hover/40 px-4 py-2 text-fc-xs text-fc-fg-muted"
	>
		<div class="flex items-center gap-2">
			<span class="font-semibold uppercase tracking-wider">Flowchart</span>
			{#if error}
				<span
					class="rounded bg-red-500/10 px-1.5 py-0.5 text-[0.65rem] font-medium text-red-400"
				>
					Syntax Error
				</span>
			{/if}
		</div>
		<div class="flex items-center gap-1.5">
			<Button variant="ghost" size="sm" onclick={() => (showSource = !showSource)}>
				{showSource ? 'View Diagram' : 'View Source'}
			</Button>
			<Button variant="ghost" size="sm" icon={icons.copy} onclick={() => copyToClipboard(code)}>
				Copy
			</Button>
		</div>
	</div>

	{#if showSource || error}
		<pre
			class="overflow-x-auto p-4 font-fc-mono text-fc-xs leading-relaxed text-fc-fg"><code>{code}</code></pre>
		{#if error}
			<div class="border-t border-fc-border bg-red-500/5 p-3 font-fc-mono text-fc-xs text-red-400">
				{error}
			</div>
		{/if}
	{:else}
		<div class="flex min-h-[120px] items-center justify-center overflow-x-auto p-6 text-fc-fg">
			{#if loading}
				<div class="flex items-center gap-2 text-fc-xs text-fc-fg-muted">
					<Spinner size="sm" /> Rendering diagram…
				</div>
			{:else if svgHtml}
				<div class="flex w-full justify-center [&>svg]:h-auto [&>svg]:max-w-full">
					{@html svgHtml}
				</div>
			{/if}
		</div>
	{/if}
</Card>
