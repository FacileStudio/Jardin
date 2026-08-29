<script lang="ts">
	import { Button, Card, ConfirmModal, Spinner, Textarea, icons, toast } from '@facile/muse';

	let {
		title,
		path,
		icon,
		backHref,
		backLabel,
		content,
		loading = false,
		deleteTitle,
		deleteDescription,
		onSave,
		onDelete
	}: {
		title: string;
		path: string;
		icon: string;
		backHref: string;
		backLabel: string;
		content: string;
		loading?: boolean;
		deleteTitle: string;
		deleteDescription: string;
		onSave: (next: string) => Promise<void>;
		onDelete: () => Promise<void>;
	} = $props();

	let editing = $state(false);
	let draft = $state('');
	let saving = $state(false);
	let confirmOpen = $state(false);

	/* Navigating between two documents keeps this component mounted, so an unfinished edit
	   would otherwise be carried over onto the next file and saved into it. */
	$effect(() => {
		path;
		editing = false;
	});

	function edit() {
		draft = content;
		editing = true;
	}

	async function save() {
		saving = true;
		try {
			await onSave(draft);
			editing = false;
			toast.success('Saved.');
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not save this file.', {
				title: 'Save failed'
			});
		} finally {
			saving = false;
		}
	}

	async function remove() {
		try {
			await onDelete();
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not delete this file.', {
				title: 'Delete failed'
			});
			throw e;
		}
	}
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

		<div class="flex flex-wrap items-start justify-between gap-4">
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

			<div class="flex shrink-0 flex-wrap items-center gap-2">
				{#if editing}
					<Button variant="ghost" onclick={() => (editing = false)} disabled={saving}>
						Cancel
					</Button>
					<Button icon={icons.check} onclick={save} disabled={saving}>
						{saving ? 'Saving…' : 'Save'}
					</Button>
				{:else}
					<Button
						variant="ghost-danger"
						icon={icons.remove}
						onclick={() => (confirmOpen = true)}
						disabled={loading}
					>
						Delete
					</Button>
					<Button icon={icons.edit} onclick={edit} disabled={loading}>Edit</Button>
				{/if}
			</div>
		</div>
	</div>

	{#if loading}
		<div class="flex items-center gap-3 text-fc-sm text-fc-fg-muted">
			<Spinner size="sm" label="Loading" /> Loading…
		</div>
	{:else if editing}
		<Textarea
			bind:value={draft}
			spellcheck="false"
			class="h-[62dvh] resize-none font-fc-mono leading-relaxed"
			aria-label="{title} content"
		/>
	{:else}
		<Card class="overflow-hidden">
			<pre
				class="max-h-[62dvh] overflow-auto whitespace-pre-wrap font-fc-mono text-fc-sm leading-relaxed text-fc-fg">{content}</pre>
		</Card>
	{/if}
</div>

<ConfirmModal
	bind:open={confirmOpen}
	tone="danger"
	title={deleteTitle}
	description={deleteDescription}
	confirmLabel="Delete"
	cancelLabel="Keep it"
	onConfirm={remove}
/>
