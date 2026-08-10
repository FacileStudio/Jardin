<script lang="ts">
	import { page } from '$app/state';
	import { Divider, PageTransition, Tabs, icons } from '@facile/muse';

	let { children } = $props();

	/*
	 * The section is a real route, not local state: /settings/bus opens on Bus, reload keeps
	 * you there and Back walks the sections. Profile lives at the bare /settings because that
	 * is where the sidebar's user card points.
	 */
	const sections = [
		{ id: 'profile', label: 'Profile', icon: icons.userCircle, href: '/settings' },
		{ id: 'appearance', label: 'Appearance', icon: icons.palette, href: '/settings/appearance' },
		{ id: 'api', label: 'API', icon: icons.key, href: '/settings/api' },
		{ id: 'bus', label: 'Bus', icon: icons.plug, href: '/settings/bus' },
		{ id: 'advanced', label: 'Advanced', icon: icons.shield, href: '/settings/advanced' }
	];

	const active = $derived(
		sections.find((s) => s.href !== '/settings' && page.url.pathname.startsWith(s.href))?.id ??
			'profile'
	);
</script>

<div class="flex flex-col gap-8">
	<div class="flex flex-col gap-2">
		<h1 class="text-fc-2xl font-semibold text-fc-fg">Settings</h1>
		<p class="text-fc-sm text-fc-fg-muted">
			Your account, this instance, and everything wired to it.
		</p>
	</div>

	<!-- gap-4, not tighter: pulled under the strip the rule reads as an underline welded to
	     the active pill. -->
	<div class="flex flex-col gap-4">
		<Tabs items={sections} value={active} label="Settings sections" />
		<Divider class="my-0" />
	</div>

	<PageTransition key={active} distance={8} duration={0.25}>
		{@render children()}
	</PageTransition>
</div>
