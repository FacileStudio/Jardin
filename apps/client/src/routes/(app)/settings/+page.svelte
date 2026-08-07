<script lang="ts">
	import { getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { Button, ProfileCard, SettingsRow, SettingsSection, icons, toast } from '@facile/muse';
	import { backend, TOKEN_KEY, type AuthUser, type MyceliumStatus } from '$lib/backend';

	const status = getContext<() => MyceliumStatus | null>('status');

	let me: AuthUser | null = $state(null);
	let loggingOut = $state(false);

	$effect(() => {
		backend
			.authMe()
			.then((u) => (me = u))
			.catch(() => (me = null));
	});

	const meta = $derived.by(() => {
		const s = status?.();
		const rows: { label: string; value: string }[] = [];
		if (s?.machine) rows.push({ label: 'This machine', value: s.machine });
		if (s) rows.push({ label: 'Rules', value: `${s.rules?.length ?? 0}` });
		if (s) rows.push({ label: 'Skills', value: `${s.skills?.length ?? 0}` });
		return rows;
	});

	async function logout() {
		loggingOut = true;
		try {
			if (me) await backend.logout();
		} catch {
			/* The session cookie may already be gone server-side; the local token still has to go. */
		}
		localStorage.removeItem(TOKEN_KEY);
		toast.success('Signed out.');
		goto('/login');
	}
</script>

<div class="flex flex-col gap-10">
	<ProfileCard
		name={me?.name || me?.email || 'admin'}
		email={me?.email}
		role={me?.admin ? 'admin' : undefined}
		{meta}
	/>

	<SettingsSection
		title="Account"
		description={me
			? 'Managed by Facile SSO — change your name or email at porte.facile.studio.'
			: 'This instance is running on a shared password, so there is no account to manage.'}
	>
		<SettingsRow
			label="Log out"
			description="Ends this session in this browser. Paired machines keep syncing."
		>
			<Button variant="outline" icon={icons.logout} disabled={loggingOut} onclick={logout}>
				{loggingOut ? 'Signing out…' : 'Log out'}
			</Button>
		</SettingsRow>
	</SettingsSection>
</div>
