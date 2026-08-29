<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Card, Spinner, icons } from '@facile/muse';

	// Read-only counterpart to DocumentEditor: flows/models are executable and trust-gated
	// per machine, so an editor here is a bigger call than "show me what's synced."
	let {
		title,
		path,
		icon,
		backHref,
		backLabel,
		content,
		loading = false,
		children
	}: {
		title: string;
		path: string;
		icon: string;
		backHref: string;
		backLabel: string;
		content: string;
		loading?: boolean;
		children?: Snippet;
	} = $props();
</script>

<div class="flex flex-col gap-10">
	<div class="flex flex-col gap-4">
		<a
			href={backHref}
			class="inline-flex w-fit items-center gap-1 text-fc-sm text-fc-fg-muted transition-colors hover:text-fc-fg focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-fc-ring"
		>
			<iconify-icon icon={icons.chevronLeft} width="16" height="16" class="block"></iconify-icon>
			{backLabel}
		</a>

		<div class="flex min-w-0 items-center gap-3">
			<span
				class="flex size-10 shrink-0 items-center justify-center rounded-fc-md bg-fc-surface text-fc-fg"
			>
				<iconify-icon {icon} width="20" height="20" class="block"></iconify-icon>
			</span>
			<div class="min-w-0">
				<h1 class="truncate text-fc-xl font-semibold text-fc-fg">{title}</h1>
				<p class="truncate font-fc-mono text-fc-xs text-fc-fg-muted">{path}</p>
			</div>
		</div>
	</div>

	{#if loading}
		<div class="flex items-center gap-3 text-fc-sm text-fc-fg-muted">
			<Spinner size="sm" label="Loading" /> Loading…
		</div>
	{:else}
		{#if children}
			{@render children()}
		{/if}
		<Card class="overflow-hidden">
			<pre
				class="max-h-[62dvh] overflow-auto whitespace-pre-wrap font-fc-mono text-fc-sm leading-relaxed text-fc-fg">{content}</pre>
		</Card>
	{/if}
</div>
