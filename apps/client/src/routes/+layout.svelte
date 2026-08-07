<script lang="ts">
	import '../app.css';
	import { browser } from '$app/environment';
	import { Toaster } from '@facile/muse';
	import { applyStoredTheme } from '$lib/theme.svelte';

	let { children } = $props();

	/*
	 * muse renders <iconify-icon> custom elements; they stay inert until the element is
	 * registered. Loading it here rather than from a CDN keeps the SPA self-hosted, which is
	 * the whole point of Jardin.
	 */
	if (browser) {
		import('iconify-icon');
		applyStoredTheme();
	}
</script>

{@render children()}

<!-- One Toaster for the whole app, outside the router, so a navigation cannot unmount a
     toast mid-flight. The extra bottom padding clears MobileNav on a phone. -->
<Toaster class="pb-28 md:pb-6" />
