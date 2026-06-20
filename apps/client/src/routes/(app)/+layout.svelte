<script lang="ts">
	import Icon from '@iconify/svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { backend, type RucheStatus } from '$lib/backend';
	import { setContext } from 'svelte';

	let { children } = $props();
	let status: RucheStatus | null = $state(null);

	const nav = [
		{ label: 'Brain', href: '/brain', icon: 'solar:brain-bold-duotone' },
		{ label: 'Rules', href: '/rules', icon: 'solar:ruler-angular-bold-duotone' },
		{ label: 'Skills', href: '/skills', icon: 'solar:bolt-circle-bold-duotone' },
		{ label: 'Cells', href: '/cells', icon: 'solar:widget-5-bold-duotone' },
		{ label: 'Settings', href: '/settings', icon: 'solar:settings-bold-duotone' }
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
		<aside class="sticky top-0 hidden h-screen w-60 flex-shrink-0 flex-col border-r border-border bg-background md:flex">
			<div class="p-4">
				<a href="/brain" class="flex items-center gap-2.5">
					<Icon icon="solar:graph-new-bold-duotone" class="size-6 text-foreground" />
					<span class="text-lg font-bold tracking-tight">Ruche</span>
				</a>
				<p class="mt-1 text-xs text-muted-foreground">
					{status.active_cell || 'no cell'}
					{#if status.machine}
						<span>· {status.machine}</span>
					{/if}
				</p>
			</div>

			<nav class="flex-1 space-y-0.5 px-2">
				{#each nav as item}
					{@const active = $page.url.pathname.startsWith(item.href)}
					<a
						href={item.href}
						class="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors {active
							? 'bg-accent font-medium text-foreground'
							: 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'}"
					>
						<Icon icon={item.icon} class="size-[18px]" />
						{item.label}
					</a>
				{/each}
			</nav>

			<div class="border-t border-border p-3">
				<button
					onclick={logout}
					class="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
				>
					<Icon icon="solar:logout-2-bold-duotone" class="size-[18px]" />
					Déconnexion
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
