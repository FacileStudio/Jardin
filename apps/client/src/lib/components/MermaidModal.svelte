<script lang="ts">
	import { Button, Spinner, icons, toast } from '@facile/muse';

	let {
		open = $bindable(false),
		svgHtml = '',
		code = '',
		error = null,
		loading = false
	}: {
		open: boolean;
		svgHtml: string;
		code: string;
		error: string | null;
		loading: boolean;
	} = $props();

	let showSource = $state(false);
	let zoom = $state(1);
	let panX = $state(0);
	let panY = $state(0);
	let isDragging = $state(false);
	let startX = 0;
	let startY = 0;
	let startPanX = 0;
	let startPanY = 0;

	function getSvgDimensions(): { width: number; height: number } {
		const match = svgHtml.match(/viewBox=["']([0-9.-]+)\s+([0-9.-]+)\s+([0-9.-]+)\s+([0-9.-]+)["']/i);
		if (match) {
			const w = parseFloat(match[3]);
			const h = parseFloat(match[4]);
			if (w > 0 && h > 0) return { width: w, height: h };
		}
		return { width: 800, height: 600 };
	}

	function fitToScreen() {
		panX = 0;
		panY = 0;
		const { width, height } = getSvgDimensions();
		const availW = Math.max(window.innerWidth - 80, 200);
		const availH = Math.max(window.innerHeight - 130, 200);
		const scaleW = availW / width;
		const scaleH = availH / height;
		const best = Math.min(scaleW, scaleH);
		zoom = Math.min(Math.max(Math.round(best * 100) / 100, 0.3), 5);
	}

	$effect(() => {
		if (open) {
			fitToScreen();
			showSource = false;
			const prevOverflow = document.body.style.overflow;
			document.body.style.overflow = 'hidden';
			window.addEventListener('keydown', handleKeydown);
			return () => {
				document.body.style.overflow = prevOverflow;
				window.removeEventListener('keydown', handleKeydown);
			};
		}
	});

	function handleKeydown(e: KeyboardEvent) {
		if (!open) return;
		if (e.key === 'Escape') {
			open = false;
		} else if (e.key === '+' || e.key === '=') {
			zoomIn();
		} else if (e.key === '-' || e.key === '_') {
			zoomOut();
		} else if (e.key === '0') {
			fitToScreen();
		}
	}

	function zoomIn() {
		zoom = Math.min(zoom * 1.25, 8);
	}

	function zoomOut() {
		zoom = Math.max(zoom * 0.8, 0.2);
	}

	function handleWheel(e: WheelEvent) {
		e.preventDefault();
		const factor = e.deltaY < 0 ? 1.15 : 0.87;
		zoom = Math.min(Math.max(zoom * factor, 0.2), 8);
	}

	function handlePointerDown(e: PointerEvent) {
		if (e.button !== 0) return;
		isDragging = true;
		startX = e.clientX;
		startY = e.clientY;
		startPanX = panX;
		startPanY = panY;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}

	function handlePointerMove(e: PointerEvent) {
		if (!isDragging) return;
		panX = startPanX + (e.clientX - startX);
		panY = startPanY + (e.clientY - startY);
	}

	function handlePointerUp(e: PointerEvent) {
		if (!isDragging) return;
		isDragging = false;
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
		} catch {}
	}

	function copyToClipboard(text: string) {
		navigator.clipboard.writeText(text);
		toast.success('Code copied to clipboard');
	}
</script>

{#if open}
	<div
		class="fixed inset-0 z-50 flex flex-col bg-fc-bg/80 backdrop-blur-xl select-none"
		role="dialog"
		aria-modal="true"
		aria-label="Flowchart fullscreen view"
	>
		<div
			class="flex flex-wrap items-center justify-between gap-3 border-b border-fc-border/60 bg-fc-surface/60 px-6 py-3"
		>
			<div class="flex items-center gap-3">
				<span class="text-fc-sm font-semibold text-fc-fg">Flowchart</span>
				<span class="rounded bg-fc-surface px-2 py-0.5 font-fc-mono text-[0.7rem] text-fc-fg-muted">Esc to exit</span>
			</div>

			<div class="flex flex-wrap items-center gap-2">
				<div class="flex items-center rounded-fc-md border border-fc-border bg-fc-surface p-0.5">
					<button
						type="button"
						class="flex size-7 items-center justify-center rounded text-fc-fg-muted hover:text-fc-fg transition-colors"
						onclick={zoomOut}
						aria-label="Zoom out"
					>
						<iconify-icon icon={icons.minus} width="16" height="16" class="block"></iconify-icon>
					</button>
					<button
						type="button"
						class="px-2 font-fc-mono text-fc-xs text-fc-fg hover:text-fc-primary transition-colors"
						onclick={fitToScreen}
						aria-label="Fit to screen"
					>
						{Math.round(zoom * 100)}%
					</button>
					<button
						type="button"
						class="flex size-7 items-center justify-center rounded text-fc-fg-muted hover:text-fc-fg transition-colors"
						onclick={zoomIn}
						aria-label="Zoom in"
					>
						<iconify-icon icon={icons.plus} width="16" height="16" class="block"></iconify-icon>
					</button>
				</div>

				<Button variant="ghost" size="sm" icon={icons.refresh} onclick={fitToScreen}>
					Fit
				</Button>

				<Button variant="ghost" size="sm" onclick={() => (showSource = !showSource)}>
					{showSource ? 'View Diagram' : 'View Source'}
				</Button>

				<Button variant="ghost" size="sm" icon={icons.copy} onclick={() => copyToClipboard(code)}>
					Copy
				</Button>

				<Button variant="ghost" size="sm" icon={icons.close} onclick={() => (open = false)}>
					Close
				</Button>
			</div>
		</div>

		{#if showSource || error}
			<div class="flex-1 overflow-auto p-6">
				<pre
					class="overflow-x-auto rounded-fc-lg border border-fc-border bg-fc-surface p-6 font-fc-mono text-fc-sm leading-relaxed text-fc-fg"><code>{code}</code></pre>
				{#if error}
					<div
						class="mt-4 rounded-fc-lg border border-red-500/30 bg-red-500/10 p-4 font-fc-mono text-fc-sm text-red-400"
					>
						{error}
					</div>
				{/if}
			</div>
		{:else}
			<div
				class="relative flex h-full w-full flex-1 items-center justify-center overflow-hidden p-8 cursor-grab active:cursor-grabbing touch-none"
				onpointerdown={handlePointerDown}
				onpointermove={handlePointerMove}
				onpointerup={handlePointerUp}
				onpointercancel={handlePointerUp}
				onwheel={handleWheel}
				ondblclick={fitToScreen}
				role="region"
				aria-label="Fullscreen flowchart canvas"
			>
				{#if loading}
					<div class="flex items-center gap-2 text-fc-sm text-fc-fg-muted">
						<Spinner size="sm" /> Rendering diagram…
					</div>
				{:else if svgHtml}
					<div
						class="flex items-center justify-center [&>svg]:h-auto [&>svg]:w-auto [&>svg]:max-w-none"
						style="transform: translate3d({panX}px, {panY}px, 0) scale({zoom}); transform-origin: center center; transition: {isDragging ? 'none' : 'transform 0.15s ease-out'};"
					>
						{@html svgHtml}
					</div>
				{/if}
			</div>
		{/if}
	</div>
{/if}
