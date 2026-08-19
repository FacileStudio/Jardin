<script lang="ts">
	import { page } from '$app/state';
	import { Divider, PageTransition, Tabs, icons } from '@facile/muse';

	let { children } = $props();

	const sections = [
		{ id: 'rules', label: 'Rules', icon: icons.shield, href: '/rules' },
		{ id: 'skills', label: 'Skills', icon: icons.bolt, href: '/skills' }
	];

	const active = $derived(sections.find((s) => page.url.pathname.startsWith(s.href))?.id ?? 'rules');
</script>

<div class="flex flex-col gap-8">
	<div class="flex flex-col gap-2">
		<h1 class="text-fc-2xl font-semibold text-fc-fg">Instructions</h1>
		<p class="text-fc-sm text-fc-fg-muted">
			What every agent reads before it starts working — installed into each agent's own config
			format.
		</p>
	</div>

	<div class="flex flex-col gap-4">
		<Tabs items={sections} value={active} label="Instruction sections" />
		<Divider class="my-0" />
	</div>

	<PageTransition key={active} distance={8} duration={0.25}>
		{@render children()}
	</PageTransition>
</div>
