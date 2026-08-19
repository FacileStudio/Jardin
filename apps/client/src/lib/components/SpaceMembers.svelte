<script lang="ts">
	import {
		Badge,
		Button,
		Card,
		ConfirmModal,
		Field,
		Select,
		Table,
		icons,
		toast
	} from '@facile/muse';
	import { backend, type SpaceMember, type SpaceRole, type UserInfo } from '$lib/backend';

	let {
		id,
		spaceName,
		members,
		candidates,
		isOwner,
		canManage,
		onChanged
	}: {
		id: string;
		spaceName: string | undefined;
		members: SpaceMember[];
		candidates: UserInfo[];
		isOwner: boolean;
		canManage: boolean;
		onChanged: () => Promise<void> | void;
	} = $props();

	const roleTone = { owner: 'owner', admin: 'admin', member: 'neutral' } as const;

	let newMemberEmail = $state('');
	let newMemberRole = $state<SpaceRole>('member');
	let adding = $state(false);

	let removeTarget = $state<SpaceMember | null>(null);
	let removeOpen = $state(false);

	async function addMember(event: Event) {
		event.preventDefault();
		if (!newMemberEmail) return;
		adding = true;
		try {
			await backend.spaceMemberAdd(id, newMemberEmail, newMemberRole);
			toast.success(`${newMemberEmail} added to ${spaceName ?? 'the space'}.`);
			newMemberEmail = '';
			newMemberRole = 'member';
			await onChanged();
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not add that member.');
		} finally {
			adding = false;
		}
	}

	async function changeRole(email: string, role: SpaceRole) {
		try {
			await backend.spaceMemberUpdate(id, email, role);
			toast.success(`${email} is now ${role}.`);
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not change that role.');
		} finally {
			await onChanged();
		}
	}

	async function removeMember() {
		const target = removeTarget;
		if (!target) return;
		try {
			await backend.spaceMemberRemove(id, target.email);
			toast.success(`${target.email} removed.`);
			removeTarget = null;
			await onChanged();
		} catch (e) {
			toast.danger(e instanceof Error ? e.message : 'Could not remove that member.');
			throw e;
		}
	}
</script>

<section class="flex flex-col gap-4">
	<div class="flex flex-col gap-1">
		<h2 class="text-fc-lg font-semibold text-fc-fg">Members</h2>
		<p class="text-fc-sm text-fc-fg-muted">
			Everyone here reads and writes the same memory, rules and skills.
		</p>
	</div>

	<Table>
		<thead>
			<tr>
				<th scope="col">Email</th>
				<th scope="col">Name</th>
				<th scope="col">Role</th>
				{#if canManage}
					<th scope="col" class="text-right" aria-label="Actions"></th>
				{/if}
			</tr>
		</thead>
		<tbody>
			{#each members as member (member.email)}
				<tr>
					<td class="font-medium text-fc-fg">{member.email}</td>
					<td class="text-fc-fg-muted">{member.name}</td>
					<td>
						{#if isOwner}
							<Select
								value={member.role}
								aria-label="Role for {member.email}"
								class="h-9 min-w-28"
								onchange={(e) =>
									changeRole(member.email, e.currentTarget.value as SpaceRole)}
							>
								<option value="member">member</option>
								<option value="admin">admin</option>
								<option value="owner">owner</option>
							</Select>
						{:else}
							<Badge tone={roleTone[member.role]}>{member.role}</Badge>
						{/if}
					</td>
					{#if canManage}
						<td class="text-right">
							<Button
								variant="ghost-danger"
								size="sm"
								icon={icons.remove}
								aria-label="Remove {member.email}"
								onclick={() => {
									removeTarget = member;
									removeOpen = true;
								}}
							>
								Remove
							</Button>
						</td>
					{/if}
				</tr>
			{/each}
		</tbody>
	</Table>

	{#if canManage}
		<Card class="flex flex-col gap-4">
			<p class="text-fc-sm font-medium text-fc-fg">Add a member</p>
			<form class="flex flex-col gap-3 sm:flex-row sm:items-end" onsubmit={addMember}>
				<div class="min-w-0 flex-1">
					<Field label="User" helper="Only people with a Jardin account can be added.">
						<Select bind:value={newMemberEmail} disabled={adding || candidates.length === 0}>
							<option value="" disabled>Select a user…</option>
							{#each candidates as user (user.email)}
								<option value={user.email}>
									{user.name ? `${user.name} — ${user.email}` : user.email}
								</option>
							{/each}
						</Select>
					</Field>
				</div>
				<Field label="Role">
					<Select bind:value={newMemberRole} class="min-w-32" disabled={adding}>
						<option value="member">member</option>
						<option value="admin">admin</option>
						{#if isOwner}
							<option value="owner">owner</option>
						{/if}
					</Select>
				</Field>
				<Button type="submit" icon={icons.plus} disabled={adding || !newMemberEmail}>
					{adding ? 'Adding…' : 'Add'}
				</Button>
			</form>
		</Card>
	{/if}
</section>

<ConfirmModal
	bind:open={removeOpen}
	tone="danger"
	title="Remove {removeTarget?.email ?? 'this member'}?"
	description="They lose access to this space's memory, rules and skills on their next sync. Anything they wrote stays."
	confirmLabel="Remove member"
	cancelLabel="Keep them"
	onConfirm={removeMember}
	onCancel={() => (removeTarget = null)}
/>
