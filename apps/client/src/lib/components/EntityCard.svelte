<script lang="ts">
	import { icons } from '@facile/muse';

	let {
		href,
		icon,
		title,
		meta
	}: { href: string; icon: string; title: string; meta?: string } = $props();
</script>

<a
	{href}
	class="group flex flex-col gap-4 rounded-fc-md bg-fc-component p-5 transition-colors hover:bg-fc-surface focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-fc-ring"
>
	<div class="flex items-center justify-between">
		<span
			class="flex size-9 items-center justify-center rounded-fc-sm bg-fc-surface text-fc-fg transition-colors group-hover:bg-fc-component"
		>
			<iconify-icon {icon} width="18" height="18" class="block"></iconify-icon>
		</span>
		<iconify-icon
			icon={icons.arrow}
			width="16"
			height="16"
			class="block text-fc-fg-muted transition-transform group-hover:translate-x-0.5"
		></iconify-icon>
	</div>
	<div class="min-w-0">
		<p class="truncate text-fc-sm font-medium text-fc-fg">{title}</p>
		{#if meta}
			<p class="truncate font-fc-mono text-fc-xs text-fc-fg-muted">{meta}</p>
		{/if}
	</div>
</a>
