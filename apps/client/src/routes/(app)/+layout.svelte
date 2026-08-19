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
	import { backend, TOKEN_KEY, type AuthUser, type MyceliumStatus } from '$lib/backend';
	import { getActiveSpaceId, getSpaces, setActiveSpaceId, setSpaces } from '$lib/space.svelte';
	import { setContext } from 'svelte';

	let { children } = $props();
	let status: MyceliumStatus | null = $state(null);
	let me: AuthUser | null = $state(null);
	let collapsed = $state(false);
	let scroller: HTMLElement | null = $state(null);

	const MOBILE_HIDDEN = ['/machines', '/spaces'];

	/*
	 * The unscoped tree is the *common* tree — that is what the API and its tests call it
	 * (`internal/server/server.go`, `scopeRoot`), and it is the instance owner's own data
	 * rather than a shared bucket. The switcher defaults to "Personal", which named the wrong
	 * thing; until muse v0.3.1 the label was unreachable through SideBar at all.
	 */
	const COMMON_TREE_LABEL = 'Common';

	/*
	 * No Settings row: settings is reached from the user card at the bottom of the rail and
	 * from the avatar in MobileNav. See CHARTE §14.
	 */
	/*
	 * Flows and Models share one rail entry: a model only exists to be a flow's `type:`
	 * step, so they are one destination with two tabs (see (automation)/+layout.svelte),
	 * not two rail rows fighting for the same mobile budget.
	 */
	const links = [
		{ label: 'Memory', href: '/memory', icon: icons.folder },
		{ label: 'Rules', href: '/rules', icon: icons.shield },
		{ label: 'Skills', href: '/skills', icon: icons.bolt },
		{ label: 'Automation', href: '/flows', icon: icons.plug, activeMatch: ['/flows', '/models'] },
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
		backend
			.status()
			.then((s) => (status = s))
			.catch(() => {
				if (getActiveSpaceId() !== null) {
					setActiveSpaceId(null);
					window.location.reload();
					return;
				}
				goto('/login');
			});
		backend
			.authMe()
			.then((u) => (me = u))
			.catch(() => (me = null));
		backend
			.spacesList()
			.then((s) => setSpaces(s))
			.catch(() => setSpaces([]));
	});

	/* <main> is the scroll container and sits outside PageTransition, so its scrollTop
	   survives a route change unless someone puts it back. */
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

	/*
	 * Every settings section collapses to one key: the sections have their own PageTransition
	 * inside the settings layout, and keying this one on the full path made both replay on
	 * every tab change — the page fading in behind the section fading in.
	 */
	const routeKey = $derived(onSettings ? '/settings' : page.url.pathname);
	/* $derived.by, not $derived: `me` is annotated `AuthUser | null` but initialised to null,
	   so an expression evaluated at declaration position is control-flow-narrowed to null.
	   A closure body reads the declared type instead. */
	const user = $derived.by(() => ({ name: me?.name || me?.email || 'admin' }));
	const switcherSpaces = $derived(getSpaces().map((s) => ({ id: s.id, name: s.name })));

	/*
	 * MobileNav is a fixed-width pill: six icons plus the avatar need 412px and the floor is
	 * 360px. Merging Flows+Models into one Automation entry keeps the rail at five daily
	 * destinations plus the avatar, so the pill carries all of them. Spaces stays reachable
	 * through the switcher's "Manage spaces" footer in the Topbar, and Machines from the
	 * Sessions page — those two are a URL away on mobile, not a tap.
	 */
	const mobilePages = $derived(navPages.filter((p) => !MOBILE_HIDDEN.includes(p.href)));

	function pickSpace(id: string | null) {
		if (id === getActiveSpaceId()) return;
		setActiveSpaceId(id);
		/* Every request is scoped by space_id, so the whole view is stale the moment it
		   changes — a reload is cheaper and safer than re-fetching each page by hand. */
		window.location.reload();
	}

	setContext('status', () => status);
</script>

{#if status}
	<div class="flex h-dvh w-full overflow-hidden bg-fc-page">
		<div class="hidden h-full shrink-0 p-3 md:block">
			<SideBar
				icon="solar:widget-6-bold-duotone"
				title="Mycelium"
				bind:collapsed
				pages={navPages}
				spaces={switcherSpaces}
				activeSpaceId={getActiveSpaceId()}
				onSpaceSelect={pickSpace}
				manageSpacesHref="/spaces"
				personalSpaceLabel={COMMON_TREE_LABEL}
				{user}
				userHref="/settings"
				userActive={onSettings}
				class="h-full"
			/>
		</div>

		<!-- `min-w-0` so a wide table inside scrolls itself instead of pushing the shell
		     sideways, and `overscroll-contain` because <main> is the only scroller. -->
		<main
			bind:this={scroller}
			class="min-w-0 flex-1 overflow-auto overscroll-contain pb-28 md:pb-0"
		>
			<!-- Spaces live in the rail, and the rail is desktop-only — without this header
			     there is no way to switch space on a phone at all. -->
			<Topbar class="md:hidden">
				<span class="text-fc-md font-semibold text-fc-fg">Mycelium</span>
				{#if switcherSpaces.length > 0}
					<div class="min-w-0 max-w-56 flex-1">
						<SpaceSwitcher
							spaces={switcherSpaces}
							activeId={getActiveSpaceId()}
							onSelect={pickSpace}
							manageHref="/spaces"
							personalLabel={COMMON_TREE_LABEL}
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
