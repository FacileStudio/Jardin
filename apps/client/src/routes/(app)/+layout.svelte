<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { backend, type RucheStatus } from '$lib/backend';
	import { setContext } from 'svelte';

	let { children } = $props();
	let status: RucheStatus | null = $state(null);

	const nav = [
		{ label: 'Brain', href: '/', icon: '🧠' },
		{ label: 'Rules', href: '/rules', icon: '📏' },
		{ label: 'Skills', href: '/skills', icon: '⚡' },
		{ label: 'Cells', href: '/cells', icon: '🔷' },
		{ label: 'Settings', href: '/settings', icon: '⚙️' }
	];

	$effect(() => {
		const token = localStorage.getItem('ruche.token');
		if (!token) {
			goto('/login');
			return;
		}
		backend.status().then((s) => (status = s)).catch(() => goto('/login'));
	});

	function logout() {
		localStorage.removeItem('ruche.token');
		goto('/login');
	}

	setContext('status', () => status);
	setContext('refreshStatus', async () => {
		status = await backend.status();
	});
</script>

{#if status}
	<div class="flex min-h-screen">
		<aside class="sticky top-0 hidden h-screen w-60 flex-shrink-0 flex-col border-r border-border bg-surface md:flex">
			<div class="p-4">
				<h1 class="text-lg font-semibold">Ruche</h1>
				<p class="text-xs text-muted">
					{status.active_cell || 'no cell'}
					{#if status.machine}
						<span class="text-muted">· {status.machine}</span>
					{/if}
				</p>
			</div>

			<nav class="flex-1 space-y-0.5 px-2">
				{#each nav as item}
					{@const active = item.href === '/' ? $page.url.pathname === '/' : $page.url.pathname.startsWith(item.href)}
					<a
						href={item.href}
						class="flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors {active
							? 'bg-primary/10 font-medium text-primary'
							: 'text-muted hover:bg-bg hover:text-fg'}"
					>
						<span class="text-base">{item.icon}</span>
						{item.label}
					</a>
				{/each}
			</nav>

			<div class="border-t border-border p-3">
				<button onclick={logout} class="w-full rounded-lg px-3 py-2 text-left text-sm text-muted hover:bg-bg hover:text-fg">
					Logout
				</button>
			</div>
		</aside>

		<main class="flex-1 overflow-auto">
			<div class="mx-auto max-w-4xl p-6">
				{@render children()}
			</div>
		</main>
	</div>
{/if}
