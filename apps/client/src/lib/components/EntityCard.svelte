<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Card, icons } from '@facile/muse';

	/*
	 * `mono` is not styling for its own sake: the charte reserves font-fc-mono for machine
	 * strings — paths, ids, endpoints. A memory file's path is one; a space's description is
	 * prose and must not wear it.
	 */
	/*
	 * `footer` is where provenance goes — the machine a thing came from, when it was made.
	 * It sits under a 1px rule, which CHARTE §5 allows *inside* a container even though the
	 * container itself carries none: the rule separates two blocks that belong to the same
	 * card, the way ProfileCard's meta rows do.
	 */
	let {
		href,
		icon,
		title,
		meta,
		mono = true,
		trailing,
		footer
	}: {
		href: string;
		icon: string;
		title: string;
		meta?: string;
		mono?: boolean;
		trailing?: Snippet;
		footer?: Snippet;
	} = $props();
</script>

<Card {href} class="min-w-0 flex flex-col gap-4">
	<div class="flex items-center justify-between gap-3">
		<span
			class="flex size-9 items-center justify-center rounded-fc-sm bg-fc-surface text-fc-fg transition-colors group-hover:bg-fc-component"
		>
			<iconify-icon {icon} width="18" height="18" class="block"></iconify-icon>
		</span>
		{#if trailing}
			{@render trailing()}
		{:else}
			<iconify-icon
				icon={icons.arrow}
				width="16"
				height="16"
				class="block text-fc-fg-muted transition-transform group-hover:translate-x-0.5"
			></iconify-icon>
		{/if}
	</div>
	<div class="min-w-0">
		<p class="truncate text-fc-sm font-medium text-fc-fg">{title}</p>
		{#if meta}
			<p class="truncate text-fc-xs text-fc-fg-muted {mono ? 'font-fc-mono' : ''}">{meta}</p>
		{/if}
	</div>
	{#if footer}
		<div
			class="flex min-w-0 items-center justify-between gap-3 border-t border-fc-border pt-3 text-fc-xs text-fc-fg-muted"
		>
			{@render footer()}
		</div>
	{/if}
</Card>
