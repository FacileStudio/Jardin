<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		MobileNav,
		PageTransition,
		SideBar,
		SpaceSwitcher,
		Spinner,
		Topbar,
		icons
	} from '@facile/muse';
	import { backend, TOKEN_KEY, type AuthUser, type MyceliumStatus, type Space } from '$lib/backend';
	import { getActiveSpaceId, getSpaces, setActiveSpaceId, setSpaces } from '$lib/space.svelte';
	import { setContext } from 'svelte';

	let { children } = $props();
	let status: MyceliumStatus | null = $state(null);
	let me: AuthUser | null = $state(null);
	let ready = $state(false);
	let collapsed = $state(false);
	let scroller: HTMLElement | null = $state(null);

	const MOBILE_HIDDEN = ['/machines', '/spaces'];

	const links = [
		{ label: 'Memory', href: '/memory', icon: icons.folder },
		{
			label: 'Instructions',
			href: '/rules',
			icon: icons.shield,
			activeMatch: ['/rules', '/skills']
		},
		{ label: 'Automation', href: '/flows', icon: icons.plug, activeMatch: ['/flows', '/models'] },
		{ label: 'Artifacts', href: '/artifacts', icon: icons.card, activeMatch: ['/artifacts', '/reports'] },
		{ label: 'Machines', href: '/machines', icon: icons.server },
		{ label: 'Sessions', href: '/sessions', icon: icons.history },
		{ label: 'Spaces', href: '/spaces', icon: icons.usersGroup }
	];

	$effect(() => {
		const token = localStorage.getItem(TOKEN_KEY);
		if (!token) {
			goto('/login');
			return;
		}

		(async () => {
			try {
				me = await backend.authMe();
			} catch {
				goto('/login');
				return;
			}

			let spaces: Space[] = [];
			try {
				spaces = await backend.spacesList();
				setSpaces(spaces);
			} catch {
				setSpaces([]);
			}

			const currentSpace = getActiveSpaceId();
			const hasValidSpace = currentSpace !== null && spaces.some((s) => s.id === currentSpace);

			if (!hasValidSpace) {
				if (spaces.length > 0) {
					setActiveSpaceId(spaces[0].id);
				} else {
					setActiveSpaceId(null);
					const isAllowed =
						page.url.pathname.startsWith('/spaces') || page.url.pathname.startsWith('/settings');
					if (!isAllowed) goto('/spaces');
				}
			}

			if (getActiveSpaceId() !== null) {
				try {
					status = await backend.status();
				} catch {
					status = null;
				}
			}

			ready = true;
		})();
	});

	$effect(() => {
		if (page.url.pathname) scroller?.scrollTo({ top: 0 });
	});

	const navPages = $derived(
		links.map((l) => ({
			...l,
			active: (l.activeMatch ?? [l.href]).some((prefix) => page.url.pathname.startsWith(prefix))
		}))
	);
	const onSettings = $derived(page.url.pathname.startsWith('/settings'));

	const routeKey = $derived(onSettings ? '/settings' : page.url.pathname);
	const user = $derived.by(() => ({ name: me?.name || me?.email || 'user' }));
	const switcherSpaces = $derived(getSpaces().map((s) => ({ id: s.id, name: s.name })));

	const mobilePages = $derived(navPages.filter((p) => !MOBILE_HIDDEN.includes(p.href)));

	function pickSpace(id: string | null) {
		if (!id || id === getActiveSpaceId()) return;
		setActiveSpaceId(id);
		window.location.reload();
	}

	setContext('status', () => status);
</script>

{#if ready}
	<div class="flex h-dvh w-full overflow-hidden bg-fc-page">
		<div class="hidden h-full shrink-0 p-3 md:block">
			<SideBar
				icon="solar:structure-bold-duotone"
				title="Mycelium"
				bind:collapsed
				pages={navPages}
				spaces={switcherSpaces}
				activeSpaceId={getActiveSpaceId()}
				onSpaceSelect={pickSpace}
				manageSpacesHref="/spaces"
				{user}
				userHref="/settings"
				userActive={onSettings}
				class="h-full"
			/>
		</div>

		<main
			bind:this={scroller}
			class="min-w-0 flex-1 overflow-auto overscroll-contain pb-28 md:pb-0"
		>
			<Topbar class="md:hidden">
				<span class="text-fc-md font-semibold text-fc-fg">Mycelium</span>
				{#if switcherSpaces.length > 0}
					<div class="min-w-0 max-w-56 flex-1">
						<SpaceSwitcher
							spaces={switcherSpaces}
							activeId={getActiveSpaceId()}
							onSelect={pickSpace}
							manageHref="/spaces"
						/>
					</div>
				{/if}
			</Topbar>

			<div class="mx-auto flex max-w-fc-lg flex-col gap-8 px-6 py-10 md:px-10">
				<PageTransition key={routeKey}>
					{@render children()}
				</PageTransition>
			</div>
		</main>

		<MobileNav items={mobilePages} {user} profileHref="/settings" profileActive={onSettings} />
	</div>
{:else}
	<div class="flex h-dvh w-full items-center justify-center bg-fc-page">
		<Spinner />
	</div>
{/if}
