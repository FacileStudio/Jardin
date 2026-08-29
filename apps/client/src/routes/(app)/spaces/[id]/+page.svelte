<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		Badge,
		Button,
		Card,
		ConfirmModal,
		Field,
		Input,
		Modal,
		Spinner,
		Textarea,
		icons,
		toast
	} from '@facile/muse';
	import { backend, type Space, type SpaceMember, type UserInfo } from '$lib/backend';
	import { setSpaces } from '$lib/space.svelte';
	import SpaceMembers from '$lib/components/SpaceMembers.svelte';

	const roleTone = { owner: 'owner', admin: 'admin', member: 'neutral' } as const;

	const id = $derived(page.params.id ?? '');
	let space = $state<Space | null>(null);
	let members: SpaceMember[] = $state([]);
	let users: UserInfo[] = $state([]);
	let loading = $state(true);

	let editOpen = $state(false);
	let draftName = $state('');
	let draftDescription = $state('');
	let savingEdit = $state(false);
	let editError = $state('');

	let leaveOpen = $state(false);
	let deleteOpen = $state(false);

	const isOwner = $derived(space?.role === 'owner');
	const canManage = $derived(space?.role === 'owner' || space?.role === 'admin');
	const candidates = $derived(users.filter((u) => !members.some((m) => m.email === u.email)));

	$effect(() => {
		const spaceId = id;
		loading = true;
		Promise.all([
			backend.spacesList().then((list) => {
				setSpaces(list);
				space = list.find((s) => s.id === spaceId) ?? null;
			}),
			backend.spaceMembers(spaceId).then((m) => (members = m)),
			backend
				.usersList()
				.then((u) => (users = u ?? []))
				.catch(() => (users = []))
		])
			.catch((e) => toast.danger(e instanceof Error ? e.message : 'Could not load this space.'))
			.finally(() => (loading = false));
	});

	async function refreshMembers() {
		members = await backend.spaceMembers(id);
	}

	function openEdit() {
		if (!space) return;
		draftName = space.name;
		draftDescription = space.description;
		editError = '';
		editOpen = true;
	}

	async function saveEdit(event: Event) {
		event.preventDefault();
		if (!draftName.trim()) return;
		savingEdit = true;
		editError = '';
		try {
			space = await backend.spaceUpdate(id, draftName.trim(), draftDescription.trim());
			editOpen = false;
			toast.success('Space updated.');
		} catch (e) {
			editError = e instanceof Error ? e.message : 'Update failed';
		} finally {
			savingEdit = false;
		}
	}

	async function leaveSpace() {
		try {
			await backend.spaceLeave(id);
			toast.success('You left the space.');
			goto('/spaces');
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not leave this space.');
			throw e;
		}
	}

	async function deleteSpace() {
		try {
			await backend.spaceDelete(id);
			toast.success('Space deleted.');
			goto('/spaces');
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not delete this space.');
			throw e;
		}
	}
</script>

<div class="flex flex-col gap-10">
	<a
		href="/spaces"
		class="inline-flex w-fit items-center gap-1 text-fc-sm text-fc-fg-muted transition-colors hover:text-fc-fg focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-fc-ring"
	>
		<iconify-icon icon={icons.chevronLeft} width="16" height="16" class="block"></iconify-icon>
		Spaces
	</a>

	{#if loading}
		<div class="flex items-center gap-3 text-fc-sm text-fc-fg-muted">
			<Spinner size="sm" label="Loading" /> Loading…
		</div>
	{:else if !space}
		<Card class="text-fc-sm text-fc-fg-muted">This space does not exist, or you left it.</Card>
	{:else}
		<div class="flex flex-wrap items-start justify-between gap-4">
			<div class="flex min-w-0 items-center gap-3">
				<span
					class="flex size-10 shrink-0 items-center justify-center rounded-fc-md bg-fc-surface text-fc-fg"
				>
					<iconify-icon icon={icons.usersGroup} width="20" height="20" class="block"
					></iconify-icon>
				</span>
				<div class="min-w-0">
					<div class="flex items-center gap-2">
						<h1 class="truncate text-fc-xl font-semibold text-fc-fg">{space.name}</h1>
						<Badge tone={roleTone[space.role]}>{space.role}</Badge>
					</div>
					<p class="truncate text-fc-sm text-fc-fg-muted">
						{space.description || 'No description'}
					</p>
				</div>
			</div>

			<div class="flex shrink-0 flex-wrap items-center gap-2">
				{#if canManage}
					<Button variant="outline" icon={icons.edit} onclick={openEdit}>Edit</Button>
				{/if}
				<Button variant="ghost" icon={icons.logout} onclick={() => (leaveOpen = true)}>
					Leave
				</Button>
				{#if isOwner}
					<Button variant="ghost-danger" icon={icons.remove} onclick={() => (deleteOpen = true)}>
						Delete
					</Button>
				{/if}
			</div>
		</div>

		<SpaceMembers
			{id}
			spaceName={space.name}
			{members}
			{candidates}
			{isOwner}
			{canManage}
			onChanged={refreshMembers}
		/>
	{/if}
</div>

<Modal bind:open={editOpen} title="Edit space" showClose>
	<form class="flex flex-col gap-4" onsubmit={saveEdit}>
		<Field label="Name" error={editError || undefined}>
			<Input bind:value={draftName} disabled={savingEdit} required />
		</Field>
		<Field label="Description">
			<Textarea bind:value={draftDescription} rows={3} disabled={savingEdit} />
		</Field>
		<div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
			<Button
				type="button"
				variant="ghost"
				class="w-full sm:w-auto"
				onclick={() => (editOpen = false)}
			>
				Cancel
			</Button>
			<Button
				type="submit"
				icon={icons.check}
				class="w-full sm:w-auto"
				disabled={savingEdit || draftName.trim().length === 0}
			>
				{savingEdit ? 'Saving…' : 'Save changes'}
			</Button>
		</div>
	</form>
</Modal>

<ConfirmModal
	bind:open={leaveOpen}
	tone="danger"
	title="Leave {space?.name ?? 'this space'}?"
	description="This space disappears from your machines on the next sync. An owner has to invite you back."
	confirmLabel="Leave space"
	cancelLabel="Stay"
	onConfirm={leaveSpace}
/>

<ConfirmModal
	bind:open={deleteOpen}
	tone="danger"
	title="Delete {space?.name ?? 'this space'}?"
	description="Every member loses it, and the memory, rules and skills stored in it are gone. This cannot be undone."
	confirmLabel="Delete space"
	cancelLabel="Keep it"
	onConfirm={deleteSpace}
/>
