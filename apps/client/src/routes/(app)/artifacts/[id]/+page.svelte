<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Alert, Badge, Button, Card, ConfirmModal, Spinner, icons, toast } from '@facile/muse';
	import { backend, type ArtifactDetail } from '$lib/backend';
	import MarkdownMuse from '$lib/components/MarkdownMuse.svelte';

	const id = $derived(page.params.id ?? '');
	let detail: ArtifactDetail | null = $state(null);
	let loading = $state(true);
	let error = $state('');
	let confirmOpen = $state(false);
	let deleting = $state(false);
	let viewMode: 'preview' | 'source' = $state('preview');

	$effect(() => {
		const artId = id;
		if (!artId) return;
		loading = true;
		backend
			.artifactGet(artId)
			.then((d) => (detail = d))
			.catch((e) => (error = e instanceof Error ? e.message : 'Could not load artifact.'))
			.finally(() => (loading = false));
	});

	async function remove() {
		if (!detail) return;
		deleting = true;
		try {
			await backend.artifactDelete(detail.id);
			toast.success(`Deleted artifact “${detail.title}”.`);
			goto('/artifacts');
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not delete artifact.', {
				title: 'Delete failed'
			});
		} finally {
			deleting = false;
		}
	}

	function formatDate(iso: string): string {
		try {
			const d = new Date(iso);
			return d.toLocaleString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric',
				hour: '2-digit',
				minute: '2-digit'
			});
		} catch {
			return iso;
		}
	}
</script>

<div class="flex flex-col gap-6">
	<div class="flex flex-col gap-3">
		<a
			href="/artifacts"
			class="inline-flex w-fit items-center gap-1 text-fc-sm text-fc-fg-muted transition-colors hover:text-fc-fg focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-fc-ring"
		>
			<iconify-icon icon={icons.chevronLeft} width="16" height="16" class="block"></iconify-icon>
			Artifacts
		</a>

		{#if detail}
			<div class="flex flex-wrap items-start justify-between gap-4">
				<div class="flex min-w-0 flex-col gap-1">
					<div class="flex min-w-0 flex-wrap items-center gap-2">
						<h1 class="min-w-0 truncate text-fc-2xl font-bold text-fc-fg">{detail.title}</h1>
						<Badge tone={detail.expired ? 'danger' : detail.expires ? 'neutral' : 'success'}>
							{detail.expires ? (detail.expired ? 'Expired' : 'Expires ' + new Date(detail.expires).toLocaleDateString()) : 'Pinned'}
						</Badge>
						<Badge tone="neutral">
							{detail.format === 'html' ? 'HTML' : 'Markdown'}
						</Badge>
					</div>
					<p
						class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 font-fc-mono text-fc-xs text-fc-fg-muted"
					>
						<span class="break-all">
							artifacts/{detail.id}.{detail.format === 'html' ? 'html' : 'md'}
						</span>
						<span class="shrink-0" aria-hidden="true">•</span>
						<span class="inline-flex min-w-0 items-center gap-1">
							<iconify-icon
								icon={icons.server}
								width="14"
								height="14"
								class="block shrink-0"
							></iconify-icon>
							<span class="truncate">{detail.machine}</span>
						</span>
						<span class="shrink-0" aria-hidden="true">•</span>
						<span class="shrink-0">{formatDate(detail.created)}</span>
					</p>
				</div>

				<div class="flex shrink-0 items-center gap-2">
					<div class="flex rounded-fc-md border border-fc-border bg-fc-surface p-0.5">
						<button
							type="button"
							class="rounded px-2.5 py-1 text-fc-xs font-medium transition-colors {viewMode === 'preview' ? 'bg-fc-accent text-fc-accent-fg' : 'text-fc-fg-muted hover:text-fc-fg'}"
							onclick={() => (viewMode = 'preview')}
						>
							Preview
						</button>
						<button
							type="button"
							class="rounded px-2.5 py-1 text-fc-xs font-medium transition-colors {viewMode === 'source' ? 'bg-fc-accent text-fc-accent-fg' : 'text-fc-fg-muted hover:text-fc-fg'}"
							onclick={() => (viewMode = 'source')}
						>
							Source
						</button>
					</div>

					<Button
						variant="ghost-danger"
						icon={icons.remove}
						onclick={() => (confirmOpen = true)}
						disabled={deleting}
					>
						Delete
					</Button>
				</div>
			</div>
		{/if}
	</div>

	{#if loading}
		<div class="flex items-center gap-3 text-fc-sm text-fc-fg-muted">
			<Spinner size="sm" label="Loading" /> Loading artifact…
		</div>
	{:else if error}
		<Alert tone="danger" title="Could not load artifact">{error}</Alert>
	{:else if detail}
		{#if viewMode === 'preview'}
			{#if detail.format === 'html'}
				<div class="overflow-hidden rounded-fc-lg border border-fc-border bg-fc-bg">
					<iframe
						title={detail.title}
						srcdoc={detail.content}
						sandbox="allow-same-origin allow-scripts allow-popups"
						class="h-[75dvh] w-full border-0 bg-fc-bg"
					></iframe>
				</div>
			{:else}
				<div class="py-2">
					<MarkdownMuse content={detail.content} />
				</div>
			{/if}
		{:else}
			<div class="overflow-hidden rounded-fc-lg border border-fc-border bg-fc-surface">
				<pre class="max-h-[75dvh] overflow-auto p-4 whitespace-pre-wrap font-fc-mono text-fc-sm leading-relaxed text-fc-fg">{detail.content}</pre>
			</div>
		{/if}
	{/if}
</div>

{#if detail}
	<ConfirmModal
		bind:open={confirmOpen}
		tone="danger"
		title="Delete “{detail.title}”?"
		description="The artifact will be permanently deleted from this machine and from sync."
		confirmLabel="Delete"
		cancelLabel="Keep it"
		onConfirm={remove}
	/>
{/if}
