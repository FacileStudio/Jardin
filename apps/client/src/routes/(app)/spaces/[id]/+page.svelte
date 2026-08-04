<script lang="ts">
	import Icon from '@iconify/svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { backend, type Space, type SpaceMember, type SpaceRole, type UserInfo } from '$lib/backend';
	import { setSpaces } from '$lib/space.svelte';

	let id = $derived($page.params.id ?? '');
	let space = $state<Space | null>(null);
	let members: SpaceMember[] = $state([]);
	let users: UserInfo[] = $state([]);
	let loading = $state(true);
	let error = $state('');
	let newMemberEmail = $state('');
	let newMemberRole = $state<SpaceRole>('member');

	let isOwner = $derived(space?.role === 'owner');
	let canManage = $derived(space?.role === 'owner' || space?.role === 'admin');
	let candidates = $derived(users.filter((u) => !members.some((m) => m.email === u.email)));

	$effect(() => {
		const spaceId = id;
		loading = true;
		Promise.all([
			backend.spacesList().then((list) => {
				setSpaces(list);
				space = list.find((s) => s.id === spaceId) ?? null;
			}),
			backend.spaceMembers(spaceId).then((m) => (members = m)),
			backend.usersList().then((u) => (users = u ?? [])).catch(() => (users = []))
		])
			.catch(() => {})
			.finally(() => (loading = false));
	});

	async function refreshMembers() {
		members = await backend.spaceMembers(id);
	}

	async function editSpace() {
		if (!space) return;
		const name = prompt('Space name:', space.name);
		if (!name) return;
		const description = prompt('Description (optional):', space.description) ?? '';
		error = '';
		try {
			space = await backend.spaceUpdate(id, name, description);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Update failed';
		}
	}

	async function addMember() {
		if (!newMemberEmail) return;
		error = '';
		try {
			await backend.spaceMemberAdd(id, newMemberEmail, newMemberRole);
			newMemberEmail = '';
			newMemberRole = 'member';
			await refreshMembers();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to add member';
		}
	}

	async function changeRole(email: string, role: SpaceRole) {
		error = '';
		try {
			await backend.spaceMemberUpdate(id, email, role);
			await refreshMembers();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to change role';
			await refreshMembers();
		}
	}

	async function removeMember(email: string) {
		if (!confirm(`Remove ${email} from this space?`)) return;
		error = '';
		try {
			await backend.spaceMemberRemove(id, email);
			await refreshMembers();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to remove member';
		}
	}

	async function leaveSpace() {
		if (!confirm('Leave this space?')) return;
		error = '';
		try {
			await backend.spaceLeave(id);
			goto('/spaces');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to leave space';
		}
	}

	async function deleteSpace() {
		if (!space) return;
		if (!confirm(`Delete space "${space.name}"? This cannot be undone.`)) return;
		error = '';
		try {
			await backend.spaceDelete(id);
			goto('/spaces');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete space';
		}
	}
</script>

<div class="space-y-6">
	<a href="/spaces" class="inline-flex items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground">
		<Icon icon="solar:alt-arrow-left-linear" class="size-4" />
		Spaces
	</a>

	{#if loading}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{:else if !space}
		<p class="text-sm text-muted-foreground">Space not found.</p>
	{:else}
		<div class="flex items-start justify-between gap-4">
			<div class="flex items-center gap-3">
				<div class="flex size-10 items-center justify-center rounded-xl bg-accent">
					<Icon icon="solar:users-group-rounded-linear" class="size-5 text-foreground" />
				</div>
				<div>
					<div class="flex items-center gap-2">
						<h2 class="text-xl font-semibold tracking-tight">{space.name}</h2>
						<span class="rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground">{space.role}</span>
					</div>
					<p class="text-sm text-muted-foreground">{space.description || 'No description'}</p>
				</div>
			</div>
			<div class="flex shrink-0 items-center gap-2">
				{#if canManage}
					<button onclick={editSpace} class="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent">
						<Icon icon="solar:pen-linear" class="size-4" />
						Edit
					</button>
				{/if}
				<button onclick={leaveSpace} class="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent">
					<Icon icon="solar:logout-2-linear" class="size-4" />
					Leave
				</button>
				{#if isOwner}
					<button onclick={deleteSpace} class="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-sm font-medium text-destructive transition-colors hover:bg-destructive/10">
						<Icon icon="solar:trash-bin-trash-linear" class="size-4" />
						Delete
					</button>
				{/if}
			</div>
		</div>

		{#if error}
			<p class="text-sm text-destructive">{error}</p>
		{/if}

		<section class="space-y-3">
			<h3 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Members</h3>

			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
							<th class="px-2 py-2 font-semibold">Email</th>
							<th class="px-2 py-2 font-semibold">Name</th>
							<th class="px-2 py-2 font-semibold">Role</th>
							{#if canManage}
								<th class="px-2 py-2"></th>
							{/if}
						</tr>
					</thead>
					<tbody>
						{#each members as member}
							<tr class="border-b border-border/50">
								<td class="px-2 py-2 font-medium">{member.email}</td>
								<td class="px-2 py-2 text-muted-foreground">{member.name}</td>
								<td class="px-2 py-2">
									{#if isOwner}
										<select
											value={member.role}
											onchange={(e) => changeRole(member.email, e.currentTarget.value as SpaceRole)}
											class="rounded-md border border-input bg-background px-2 py-1 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
										>
											<option value="member">member</option>
											<option value="admin">admin</option>
											<option value="owner">owner</option>
										</select>
									{:else}
										<span class="rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground">{member.role}</span>
									{/if}
								</td>
								{#if canManage}
									<td class="px-2 py-2 text-right">
										<button onclick={() => removeMember(member.email)} class="text-xs text-destructive hover:underline">
											Remove
										</button>
									</td>
								{/if}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			{#if canManage}
				<form onsubmit={(e) => { e.preventDefault(); addMember(); }} class="flex flex-wrap gap-2">
					<select
						bind:value={newMemberEmail}
						class="min-w-52 flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
					>
						<option value="" disabled>Select a user…</option>
						{#each candidates as user}
							<option value={user.email}>{user.name ? `${user.name} — ${user.email}` : user.email}</option>
						{/each}
					</select>
					<select
						bind:value={newMemberRole}
						class="rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
					>
						<option value="member">member</option>
						<option value="admin">admin</option>
						{#if isOwner}
							<option value="owner">owner</option>
						{/if}
					</select>
					<button
						type="submit"
						disabled={!newMemberEmail}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-primary px-3.5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
					>
						<Icon icon="mdi:plus" class="size-4" />
						Add
					</button>
				</form>
			{/if}
		</section>
	{/if}
</div>
