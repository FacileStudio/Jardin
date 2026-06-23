<script lang="ts">
	import Icon from '@iconify/svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import type { Action } from 'svelte/action';

	let visible = $state(false);

	onMount(() => {
		if (localStorage.getItem('ruche.token')) {
			goto('/memory');
			return;
		}
		visible = true;
	});

	const reveal: Action<HTMLElement, { delay?: number; threshold?: number }> = (node, params = {}) => {
		const { delay = 0, threshold = 0.15 } = params;

		if (typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
			node.style.opacity = '1';
			return { destroy() {} };
		}

		node.style.opacity = '0';
		node.style.transform = 'translateY(32px) scale(0.97)';
		node.style.transition = `opacity 0.8s cubic-bezier(0.16, 1, 0.3, 1) ${delay}ms, transform 0.8s cubic-bezier(0.16, 1, 0.3, 1) ${delay}ms`;

		const observer = new IntersectionObserver(
			([entry]) => {
				if (entry.isIntersecting) {
					node.style.opacity = '1';
					node.style.transform = 'translateY(0) scale(1)';
					observer.unobserve(node);
				}
			},
			{ threshold }
		);

		observer.observe(node);

		return {
			destroy() {
				observer.disconnect();
			}
		};
	};

	const card = 'group relative overflow-hidden rounded-2xl border border-zinc-200 p-6 transition-all duration-300 ease-[cubic-bezier(0.16,1,0.3,1)] hover:-translate-y-1 hover:border-zinc-300 hover:shadow-xl motion-reduce:transition-none';
	const glow = 'pointer-events-none absolute -right-20 -top-20 size-40 rounded-full bg-zinc-100 opacity-0 blur-3xl transition-opacity duration-500 group-hover:opacity-70';
	const ico = 'transition-transform duration-300 group-hover:-translate-y-0.5 group-hover:scale-110';
</script>

<svelte:head>
	<title>Ruche — Shared Agent Memory</title>
	<meta name="description" content="Un cerveau partagé pour vos agents IA. Mémoire, règles et compétences synchronisées entre Claude, Gemini, Codex et toutes vos machines." />
</svelte:head>

{#if visible}
<div class="min-h-screen bg-white text-zinc-900">
	<header class="fixed top-0 z-50 w-full border-b border-zinc-200 bg-white/90 backdrop-blur-sm">
		<div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
			<a href="/" class="flex items-center gap-2.5">
				<Icon icon="solar:graph-new-bold-duotone" class="size-7 text-zinc-900" />
				<span class="text-xl font-bold tracking-tight">Ruche</span>
			</a>
			<a
				href="/login"
				class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-zinc-800"
			>
				Se connecter
				<Icon icon="solar:arrow-right-linear" class="size-3.5" />
			</a>
		</div>
	</header>

	<main>
		<section class="mx-auto max-w-5xl px-6 pt-36 pb-28 md:pt-44 md:pb-36">
			<div class="transition-all duration-700 ease-out {visible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-4'}">
				<p class="mb-6 inline-flex items-center gap-2 rounded-full border border-zinc-200 px-3.5 py-1 text-xs text-zinc-500">
					<Icon icon="solar:server-bold-duotone" class="size-3.5" />
					Local-first &middot; Multi-agent &middot; Open source
				</p>
				<h1 class="max-w-3xl text-5xl leading-[1.08] font-black tracking-tight md:text-7xl">
					Un cerveau.<br />
					<span class="text-zinc-400">Tous vos agents.</span>
				</h1>
				<p class="mt-8 max-w-lg text-lg leading-relaxed text-zinc-500">
					Mémoire, règles et compétences partagées entre Claude, Gemini, Codex, Cursor et toutes vos machines. Un seul endroit, zéro friction.
				</p>
				<div class="mt-10 flex flex-wrap items-center gap-4">
					<a
						href="/login"
						class="inline-flex items-center gap-2 rounded-md bg-zinc-900 px-6 py-3 text-base font-medium text-white transition-colors hover:bg-zinc-800"
					>
						Commencer
						<Icon icon="solar:arrow-right-linear" class="size-4" />
					</a>
					<a
						href="https://github.com/FacileStudio/Ruche"
						target="_blank"
						rel="noopener noreferrer"
						class="inline-flex items-center gap-2 rounded-md border border-zinc-200 px-6 py-3 text-base font-medium text-zinc-600 transition-colors hover:border-zinc-400 hover:text-zinc-900"
					>
						GitHub
						<Icon icon="solar:arrow-right-up-linear" class="size-3.5" />
					</a>
				</div>
			</div>
		</section>

		<section class="border-y border-zinc-200 bg-zinc-950 text-white">
			<div class="mx-auto max-w-5xl px-6 py-28 md:py-36">
				<div use:reveal={{ delay: 0 }} class="mb-10 flex items-center gap-3">
					<Icon icon="solar:code-square-bold-duotone" class="size-8" />
					<span class="text-2xl font-semibold tracking-tight">Adapters</span>
				</div>
				<h2 use:reveal={{ delay: 100 }} class="max-w-2xl text-4xl leading-[1.1] font-black tracking-tight md:text-6xl">
					Écrivez une fois.<br />
					<span class="text-zinc-500">Déployez partout.</span>
				</h2>
				<p use:reveal={{ delay: 200 }} class="mt-8 max-w-xl text-lg leading-relaxed text-zinc-400">
					Vos règles et compétences sont écrites en markdown. Ruche génère automatiquement les fichiers de configuration pour chaque agent.
				</p>

				<div use:reveal={{ delay: 300 }} class="mt-16 grid gap-4 sm:grid-cols-3">
					{#each [
						{ agent: 'Claude Code', file: 'CLAUDE.md', icon: 'solar:chat-square-code-bold-duotone', color: 'text-orange-400' },
						{ agent: 'Gemini CLI', file: 'GEMINI.md', icon: 'solar:stars-bold-duotone', color: 'text-blue-400' },
						{ agent: 'Codex', file: 'AGENTS.md', icon: 'solar:programming-bold-duotone', color: 'text-green-400' },
						{ agent: 'Cursor', file: '.cursor/rules/', icon: 'solar:cursor-bold-duotone', color: 'text-purple-400' },
						{ agent: 'Copilot', file: 'copilot-instructions.md', icon: 'solar:ghost-bold-duotone', color: 'text-sky-400' },
						{ agent: 'Hermes', file: 'SOUL.md', icon: 'solar:bolt-circle-bold-duotone', color: 'text-red-400' },
					] as item}
						<div class="rounded-xl border border-zinc-800 bg-zinc-900/50 px-4 py-3">
							<div class="flex items-center gap-2">
								<Icon icon={item.icon} class="size-4 {item.color}" />
								<p class="text-sm font-medium {item.color}">{item.agent}</p>
							</div>
							<p class="mt-1 text-xs text-zinc-500 font-mono">{item.file}</p>
						</div>
					{/each}
				</div>
			</div>
		</section>

		<section class="mx-auto max-w-5xl px-6 py-28 md:py-36">
			<div use:reveal={{ delay: 0 }} class="mb-16 max-w-lg">
				<h2 class="text-4xl font-black tracking-tight md:text-5xl">Trois couches.<br /><span class="text-zinc-400">Chacune indépendante.</span></h2>
				<p class="mt-4 text-zinc-500">Pas de dépendance. Chaque pièce fonctionne seule et s'enrichit avec les autres.</p>
			</div>

			<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
				<div use:reveal={{ delay: 0 }} class="{card}">
					<div class={glow}></div>
					<div class="mb-4 {ico}">
						<Icon icon="solar:notebook-bold-duotone" class="size-6 text-zinc-900" />
					</div>
					<h3 class="text-lg font-bold tracking-tight">Memory</h3>
					<p class="mt-2 text-sm leading-relaxed text-zinc-500">
						Wiki partagé en markdown. Bugs, outils, projets, conventions — vos agents apprennent de chaque session et partagent ce savoir entre eux.
					</p>
				</div>

				<div use:reveal={{ delay: 100 }} class="{card}">
					<div class={glow}></div>
					<div class="mb-4 {ico}">
						<Icon icon="solar:ruler-angular-bold-duotone" class="size-6 text-zinc-900" />
					</div>
					<h3 class="text-lg font-bold tracking-tight">Rules</h3>
					<p class="mt-2 text-sm leading-relaxed text-zinc-500">
						Règles modulaires pour vos agents. Style de code, conventions git, engineering ladder — écrivez-les une fois, tous vos agents les suivent.
					</p>
				</div>

				<div use:reveal={{ delay: 200 }} class="{card}">
					<div class={glow}></div>
					<div class="mb-4 {ico}">
						<Icon icon="solar:bolt-circle-bold-duotone" class="size-6 text-zinc-900" />
					</div>
					<h3 class="text-lg font-bold tracking-tight">Skills</h3>
					<p class="mt-2 text-sm leading-relaxed text-zinc-500">
						Compétences agent-agnostiques avec des définitions portables. Un skill, six agents. Pas de vendor lock-in.
					</p>
				</div>
			</div>
		</section>

		<section class="border-y border-zinc-200 bg-zinc-50">
			<div class="mx-auto max-w-5xl px-6 py-28 md:py-36">
				<div class="grid items-center gap-16 md:grid-cols-2">
					<div>
						<div use:reveal={{ delay: 0 }} class="mb-10 flex items-center gap-3">
							<Icon icon="solar:widget-5-bold-duotone" class="size-8 text-zinc-900" />
							<span class="text-2xl font-semibold tracking-tight">Cells</span>
						</div>
						<h2 use:reveal={{ delay: 100 }} class="text-4xl font-black tracking-tight md:text-5xl">
							Perso vs. équipe.
						</h2>
						<p use:reveal={{ delay: 200 }} class="mt-8 max-w-lg text-lg leading-relaxed text-zinc-500">
							Un profil personnel, un profil équipe, un profil client — chaque cell a son propre memory, ses règles et ses skills. Superposez-les : les règles perso gagnent, la mémoire s'additionne.
						</p>
					</div>
					<div use:reveal={{ delay: 300 }} class="grid gap-4">
						{#each [
							{ name: 'personal', desc: 'Votre mémoire, vos préférences, vos raccourcis', active: true },
							{ name: 'facile', desc: 'Conventions d\'équipe, stack technique, projets partagés', active: false },
							{ name: 'client-x', desc: 'Contexte spécifique au projet, règles du client', active: false },
						] as cell}
							<div class="rounded-xl border px-5 py-4 transition-all duration-300 hover:-translate-y-0.5 hover:shadow-lg {cell.active ? 'border-zinc-900 bg-zinc-950 text-white' : 'border-zinc-200 bg-white'}">
								<div class="flex items-center gap-2">
									<Icon icon={cell.active ? 'solar:check-circle-bold-duotone' : 'solar:circle-bold-duotone'} class="size-4 {cell.active ? 'text-white' : 'text-zinc-300'}" />
									<p class="text-sm font-semibold">{cell.name}</p>
								</div>
								<p class="mt-1 text-xs {cell.active ? 'text-zinc-400' : 'text-zinc-500'}">{cell.desc}</p>
							</div>
						{/each}
					</div>
				</div>
			</div>
		</section>

		<section class="bg-zinc-950 text-white">
			<div class="mx-auto max-w-5xl px-6 py-28 md:py-36">
				<div use:reveal={{ delay: 0 }} class="mb-10 flex items-center gap-3">
					<Icon icon="solar:refresh-circle-bold-duotone" class="size-8" />
					<span class="text-2xl font-semibold tracking-tight">Sync</span>
				</div>
				<h2 use:reveal={{ delay: 100 }} class="text-4xl font-black tracking-tight md:text-5xl">
					Sync intégré.<br />
					<span class="text-zinc-500">Même binaire.</span>
				</h2>
				<p use:reveal={{ delay: 200 }} class="mt-8 max-w-xl text-lg leading-relaxed text-zinc-400">
					<code class="rounded bg-zinc-800 px-2 py-0.5 text-sm font-mono">ruche serve</code> lance un serveur de sync HTTP. Déployez-le sur votre VPS, connectez vos machines avec un token.
				</p>

				<div use:reveal={{ delay: 300 }} class="mt-12 rounded-xl border border-zinc-800 bg-zinc-900/50 p-6 font-mono text-sm">
					<p class="text-zinc-500"># sur le serveur</p>
					<p class="text-green-400">$ ruche serve --port 8420</p>
					<p class="mt-4 text-zinc-500"># sur votre machine</p>
					<p class="text-green-400">$ ruche sync</p>
					<p class="text-zinc-600 mt-1">&nbsp; ↓ memory/tools/dokploy.md</p>
					<p class="text-zinc-600">&nbsp; ↑ rules/engineering-ladder.md</p>
					<p class="text-zinc-600">&nbsp; Synced 2 file(s).</p>
				</div>
			</div>
		</section>

		<section class="border-t border-zinc-200">
			<div class="mx-auto max-w-5xl px-6 py-28 md:py-36 text-center">
				<h2 use:reveal={{ delay: 0 }} class="text-3xl font-bold tracking-tight">
					Open source. Local-first. Gratuit.
				</h2>
				<p use:reveal={{ delay: 100 }} class="mt-4 text-zinc-500">
					Un binaire Go. Zéro dépendance. Vos données restent chez vous.
				</p>
				<div class="mt-10 flex justify-center gap-3">
					<a
						href="/login"
						class="inline-flex h-11 items-center justify-center rounded-md bg-zinc-900 px-6 text-sm font-medium text-white transition-colors hover:bg-zinc-800"
					>
						Se connecter
					</a>
					<a
						href="https://github.com/FacileStudio/Ruche"
						target="_blank"
						rel="noopener noreferrer"
						class="inline-flex h-11 items-center justify-center rounded-md border border-zinc-200 px-6 text-sm font-medium transition-colors hover:bg-zinc-50"
					>
						Voir le code
						<Icon icon="solar:arrow-right-up-linear" class="ml-2 size-3.5" />
					</a>
				</div>
			</div>
		</section>
	</main>

	<footer class="border-t border-zinc-200">
		<div class="mx-auto max-w-5xl px-6 py-6 text-center text-sm text-zinc-400">
			&copy; {new Date().getFullYear()} Ruche by <a href="https://facile.studio" class="text-zinc-600 transition-colors hover:text-zinc-900">Facile.</a>
		</div>
	</footer>
</div>
{/if}
