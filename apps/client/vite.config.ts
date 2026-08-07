import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	/*
	 * muse ships uncompiled source, including `.svelte.ts` rune modules. Vite's dev-only
	 * dependency optimizer hands those to esbuild without the TypeScript transform, so
	 * `utils/toast.svelte.ts` dies on its first type annotation and `vite dev` refuses to
	 * start — while `vite build`, which never runs the optimizer, is perfectly happy.
	 * Excluding the package leaves it to the svelte plugin, which is what compiles it.
	 */
	optimizeDeps: {
		exclude: ['@facile/muse']
	}
});
