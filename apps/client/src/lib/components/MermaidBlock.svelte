<script lang="ts">
	import { Button, Card, Spinner, icons, toast } from '@facile/muse';
	import mermaid from 'mermaid';
	import MermaidModal from './MermaidModal.svelte';

	let { code = '' }: { code: string } = $props();

	let svgHtml = $state('');
	let error = $state<string | null>(null);
	let loading = $state(true);
	let showSource = $state(false);
	let isFullscreen = $state(false);

	let inlineZoom = $state(1.2);
	let inlinePanX = $state(0);
	let inlinePanY = $state(0);
	let inlineDragging = $state(false);
	let inlineStartX = 0;
	let inlineStartY = 0;
	let inlineStartPanX = 0;
	let inlineStartPanY = 0;

	function getSvgDimensions(): { width: number; height: number } {
		const match = svgHtml.match(/viewBox=["']([0-9.-]+)\s+([0-9.-]+)\s+([0-9.-]+)\s+([0-9.-]+)["']/i);
		if (match) {
			const w = parseFloat(match[3]);
			const h = parseFloat(match[4]);
			if (w > 0 && h > 0) return { width: w, height: h };
		}
		return { width: 800, height: 600 };
	}

	function defaultInlineZoom(): number {
		const { width } = getSvgDimensions();
		if (width <= 350) return 1.5;
		if (width <= 550) return 1.35;
		if (width <= 800) return 1.2;
		return 1.05;
	}

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
			svgHtml = result.svg.replace(/style="([^"]*?)max-width:\s*[^;"]+;?([^"]*?)"/gi, 'style="$1$2"');
			inlineZoom = defaultInlineZoom();
			inlinePanX = 0;
			inlinePanY = 0;
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

	function zoomInInline() {
		inlineZoom = Math.min(inlineZoom * 1.25, 6);
	}

	function zoomOutInline() {
		inlineZoom = Math.max(inlineZoom * 0.8, 0.2);
	}

	function resetInlineZoom() {
		inlineZoom = defaultInlineZoom();
		inlinePanX = 0;
		inlinePanY = 0;
	}

	function handleInlineWheel(e: WheelEvent) {
		e.preventDefault();
		const factor = e.deltaY < 0 ? 1.15 : 0.87;
		inlineZoom = Math.min(Math.max(inlineZoom * factor, 0.2), 6);
	}

	function handleInlinePointerDown(e: PointerEvent) {
		if (e.button !== 0) return;
		inlineDragging = true;
		inlineStartX = e.clientX;
		inlineStartY = e.clientY;
		inlineStartPanX = inlinePanX;
		inlineStartPanY = inlinePanY;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}

	function handleInlinePointerMove(e: PointerEvent) {
		if (!inlineDragging) return;
		inlinePanX = inlineStartPanX + (e.clientX - inlineStartX);
		inlinePanY = inlineStartPanY + (e.clientY - inlineStartY);
	}

	function handleInlinePointerUp(e: PointerEvent) {
		if (!inlineDragging) return;
		inlineDragging = false;
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
		} catch {}
	}
</script>

<Card class="my-4 overflow-hidden p-0">
	<div
		class="flex flex-wrap items-center justify-between gap-2 border-b border-fc-border bg-fc-surface-hover/40 px-4 py-2 text-fc-xs text-fc-fg-muted"
	>
		<div class="flex items-center gap-2">
			<span class="font-semibold uppercase tracking-wider">Flowchart</span>
			{#if error}
				<span
					class="rounded bg-fc-danger/10 px-1.5 py-0.5 text-[0.65rem] font-medium text-fc-danger"
				>
					Syntax Error
				</span>
			{/if}
		</div>
		<div class="flex flex-wrap items-center gap-1.5">
			{#if !showSource && !error && svgHtml}
				<div class="flex items-center rounded-fc-md border border-fc-border bg-fc-surface p-0.5">
					<button
						type="button"
						class="flex size-6 items-center justify-center rounded text-fc-fg-muted hover:text-fc-fg transition-colors"
						onclick={zoomOutInline}
						aria-label="Zoom out"
					>
						<iconify-icon icon={icons.minus} width="14" height="14" class="block"></iconify-icon>
					</button>
					<button
						type="button"
						class="px-1.5 font-fc-mono text-[0.7rem] text-fc-fg hover:text-fc-primary transition-colors"
						onclick={resetInlineZoom}
						aria-label="Reset zoom"
					>
						{Math.round(inlineZoom * 100)}%
					</button>
					<button
						type="button"
						class="flex size-6 items-center justify-center rounded text-fc-fg-muted hover:text-fc-fg transition-colors"
						onclick={zoomInInline}
						aria-label="Zoom in"
					>
						<iconify-icon icon={icons.plus} width="14" height="14" class="block"></iconify-icon>
					</button>
				</div>

				<Button variant="ghost" size="sm" onclick={() => (isFullscreen = true)}>
					<iconify-icon icon="solar:maximize-square-linear" width="14" height="14" class="mr-1 block"></iconify-icon>
					Fullscreen
				</Button>
			{/if}

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
			<div class="border-t border-fc-border bg-fc-danger/5 p-3 font-fc-mono text-fc-xs text-fc-danger">
				{error}
			</div>
		{/if}
	{:else}
		<div
			class="relative flex min-h-[160px] w-full items-center justify-center overflow-hidden p-6 text-fc-fg select-none cursor-grab active:cursor-grabbing touch-none"
			onpointerdown={handleInlinePointerDown}
			onpointermove={handleInlinePointerMove}
			onpointerup={handleInlinePointerUp}
			onpointercancel={handleInlinePointerUp}
			onwheel={handleInlineWheel}
			ondblclick={() => (isFullscreen = true)}
			role="region"
			aria-label="Interactive flowchart diagram"
		>
			{#if loading}
				<div class="flex items-center gap-2 text-fc-xs text-fc-fg-muted">
					<Spinner size="sm" label="Loading" /> Rendering diagram…
				</div>
			{:else if svgHtml}
				<div
					class="flex items-center justify-center [&>svg]:h-auto [&>svg]:w-auto [&>svg]:max-w-none"
					style="transform: translate3d({inlinePanX}px, {inlinePanY}px, 0) scale({inlineZoom}); transform-origin: center center; transition: {inlineDragging ? 'none' : 'transform 0.15s ease-out'};"
				>
					{@html svgHtml}
				</div>
			{/if}
		</div>
	{/if}
</Card>

<MermaidModal
	bind:open={isFullscreen}
	{svgHtml}
	{code}
	{error}
	{loading}
/>
