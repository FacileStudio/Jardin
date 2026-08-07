<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Card } from '@facile/muse';

	let {
		icon,
		title,
		description,
		children
	}: { icon: string; title: string; description?: string; children?: Snippet } = $props();
</script>

<Card class="flex flex-col items-center gap-3 py-12 text-center">
	<iconify-icon {icon} width="24" height="24" class="block text-fc-fg-muted"></iconify-icon>
	<p class="text-fc-sm font-medium text-fc-fg">{title}</p>
	{#if description}
		<p class="max-w-sm text-fc-sm text-fc-fg-muted">{description}</p>
	{/if}
	{#if children}{@render children()}{/if}
</Card>
