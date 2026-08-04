<script lang="ts">
	import Icon from '@iconify/svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { backend, type AuthUser, type JardinStatus } from '$lib/backend';
	import { getActiveSpaceId, getSpaces, setActiveSpaceId, setSpaces } from '$lib/space.svelte';
	import { setContext } from 'svelte';

	let { children } = $props();
	let status: JardinStatus | null = $state(null);
	let me: AuthUser | null = $state(null);
	let spaceMenuOpen = $state(false);

	const nav = [
		{ label: 'Memory', href: '/memory', icon: 'solar:notebook-linear' },
		{ label: 'Rules', href: '/rules', icon: 'solar:ruler-angular-linear' },
		{ label: 'Skills', href: '/skills', icon: 'solar:bolt-circle-linear' },
		{ label: 'Machines', href: '/machines', icon: 'solar:server-square-linear' },
		{ label: 'Sessions', href: '/sessions', icon: 'solar:history-linear' },
		{ label: 'Spaces', href: '/spaces', icon: 'solar:users-group-rounded-linear' },
		{ label: 'Settings', href: '/settings', icon: 'solar:settings-linear' }
	];

	$effect(() => {
		const token = localStorage.getItem('jardin.token');
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
		backend.authMe().then((u) => (me = u)).catch(() => (me = null));
		backend.spacesList().then((s) => setSpaces(s)).catch(() => setSpaces([]));
	});

	let activeSpace = $derived(getSpaces().find((s) => s.id === getActiveSpaceId()) ?? null);

	function pickSpace(id: string | null) {
		spaceMenuOpen = false;
		if (id === getActiveSpaceId()) return;
		setActiveSpaceId(id);
		window.location.reload();
	}

	async function logout() {
		if (me) {
			try {
				await backend.logout();
			} catch {}
		}
		localStorage.removeItem('jardin.token');
		goto('/login');
	}

	setContext('status', () => status);
</script>

{#if status}
	<div class="flex min-h-screen">
		<aside class="sticky top-0 hidden h-screen w-60 flex-shrink-0 flex-col border-r border-border bg-background md:flex">
			<div class="p-4">
				<a href="/memory" class="flex items-center gap-2.5">
					<Icon icon="solar:leaf-bold-duotone" class="size-6 text-foreground" />
					<span class="text-lg font-bold tracking-tight">Jardin</span>
				</a>
			</div>

			{#if getSpaces().length > 0}
				<div class="relative px-2 pb-2">
					<button
						onclick={() => (spaceMenuOpen = !spaceMenuOpen)}
						class="flex w-full items-center gap-2.5 rounded-lg border border-border px-3 py-2 text-sm transition-colors hover:bg-accent"
					>
						<Icon
							icon={activeSpace ? 'solar:planet-linear' : 'solar:users-group-rounded-linear'}
							class="size-[18px] text-muted-foreground"
						/>
						<span class="flex-1 truncate text-left font-medium">{activeSpace ? activeSpace.name : 'Common'}</span>
						<Icon icon="solar:alt-arrow-down-linear" class="size-4 text-muted-foreground" />
					</button>

					{#if spaceMenuOpen}
						<button
							class="fixed inset-0 z-10 cursor-default"
							onclick={() => (spaceMenuOpen = false)}
							aria-label="Close space menu"
						></button>
						<div class="absolute left-2 right-2 top-full z-20 mt-1 rounded-lg border border-border bg-background p-1 shadow-md">
							<button
								onclick={() => pickSpace(null)}
								class="flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent {getActiveSpaceId() === null
									? 'bg-accent font-medium text-foreground'
									: 'text-muted-foreground'}"
							>
								<Icon icon="solar:users-group-rounded-linear" class="size-[18px]" />
								Common
							</button>
							{#each getSpaces() as space}
								<button
									onclick={() => pickSpace(space.id)}
									class="flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent {getActiveSpaceId() === space.id
										? 'bg-accent font-medium text-foreground'
										: 'text-muted-foreground'}"
								>
									<Icon icon="solar:planet-linear" class="size-[18px]" />
									<span class="truncate">{space.name}</span>
								</button>
							{/each}
						</div>
					{/if}
				</div>
			{/if}

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
				<div class="flex items-center gap-2.5 px-3 py-2">
					<div class="min-w-0 flex-1">
						<p class="truncate text-sm font-medium">{me ? me.name || me.email : 'admin'}</p>
						{#if me}
							<p class="truncate text-xs text-muted-foreground">{me.email}</p>
						{/if}
					</div>
					<button
						onclick={logout}
						title="Déconnexion"
						class="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
					>
						<Icon icon="solar:logout-2-linear" class="size-[18px]" />
					</button>
				</div>
			</div>
		</aside>

		<main class="flex-1 overflow-auto">
			<div class="mx-auto max-w-5xl p-6 md:p-8">
				{@render children()}
			</div>
		</main>
	</div>
{/if}
