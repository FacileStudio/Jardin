<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { Button, Card, WordReveal, icons } from '@facile/muse';
	import { TOKEN_KEY } from '$lib/backend';

	let visible = $state(false);

	onMount(() => {
		if (localStorage.getItem(TOKEN_KEY)) {
			goto('/memory');
			return;
		}
		visible = true;
	});

	/* Inside an inverted band nothing may read `fc-fg`: the ink there is `fc-accent-fg`. */
	const invertedDim = 'color-mix(in oklab, var(--color-fc-accent-fg) 25%, transparent)';
	const invertedInk = 'var(--color-fc-accent-fg)';

	const adapters = [
		{ agent: 'pi', file: 'SOUL.md', icon: 'solar:widget-6-bold-duotone' },
		{ agent: 'Claude Code', file: 'CLAUDE.md', icon: 'simple-icons:anthropic' },
		{ agent: 'Gemini CLI', file: 'GEMINI.md', icon: 'simple-icons:googlegemini' },
		{ agent: 'Codex', file: 'AGENTS.md', icon: 'simple-icons:openai' },
		{ agent: 'Cursor', file: '.cursor/rules/', icon: 'simple-icons:cursor' },
		{ agent: 'Copilot', file: 'copilot-instructions.md', icon: 'simple-icons:githubcopilot' }
	];

	const layers = [
		{
			icon: icons.folder,
			title: 'Memory',
			body: 'Wiki partagé en markdown. Bugs, outils, projets, conventions — vos agents apprennent de chaque session et partagent ce savoir entre eux.'
		},
		{
			icon: icons.shield,
			title: 'Rules',
			body: 'Règles modulaires pour vos agents. Style de code, conventions git, engineering ladder — écrivez-les une fois, tous vos agents les suivent.'
		},
		{
			icon: icons.bolt,
			title: 'Skills',
			body: 'Compétences agent-agnostiques avec des définitions portables. Un skill, six agents. Pas de vendor lock-in.'
		}
	];

	const cells = [
		{ name: 'personal', desc: 'Votre mémoire, vos préférences, vos raccourcis', active: true },
		{
			name: 'facile',
			desc: "Conventions d'équipe, stack technique, projets partagés",
			active: false
		},
		{ name: 'client-x', desc: 'Contexte spécifique au projet, règles du client', active: false }
	];
</script>

<svelte:head>
	<title>Jardin — Shared Agent Memory</title>
	<meta
		name="description"
		content="Un cerveau partagé pour vos agents IA. Mémoire, règles et compétences synchronisées entre pi, Claude, Gemini, Codex et toutes vos machines."
	/>
</svelte:head>

{#if visible}
	<div class="min-h-dvh bg-fc-page text-fc-fg">
		<header
			class="fixed top-0 z-50 w-full border-b border-fc-border bg-fc-page/90 backdrop-blur"
		>
			<div class="mx-auto flex max-w-fc-lg items-center justify-between px-6 py-4">
				<a href="/" class="flex items-center gap-2.5">
					<iconify-icon icon="solar:widget-6-bold-duotone" width="24" height="24" class="block"
					></iconify-icon>
					<span class="text-fc-xl font-semibold text-fc-fg">Jardin</span>
				</a>
				<Button href="/login" iconRight={icons.arrow} class="rounded-fc-md">Se connecter</Button>
			</div>
		</header>

		<main>
			<section class="mx-auto max-w-fc-lg px-6 pb-28 pt-36 md:pb-36 md:pt-44">
				<p
					class="mb-6 inline-flex items-center gap-2 rounded-fc-pill border border-fc-border px-3.5 py-1 text-fc-xs text-fc-fg-muted"
				>
					<iconify-icon icon={icons.server} width="14" height="14" class="block"></iconify-icon>
					Local-first · Multi-agent · Open source
				</p>
				<h1 class="max-w-3xl text-fc-3xl font-semibold leading-tight md:text-6xl">
					Un cerveau.<br />
					<span class="text-fc-fg-muted">Tous vos agents.</span>
				</h1>
				<p class="mt-8 max-w-lg text-fc-md leading-relaxed text-fc-fg-muted">
					Mémoire, règles et compétences partagées entre pi, Claude, Gemini, Codex, Cursor et toutes
					vos machines. Un seul endroit, zéro friction.
				</p>
				<div class="mt-10 flex flex-wrap items-center gap-4">
					<Button href="/login" size="lg" iconRight={icons.arrow} class="rounded-fc-md">Commencer</Button>
					<Button
						href="https://github.com/FacileStudio/Jardin"
						target="_blank"
						rel="noopener noreferrer"
						variant="outline"
						size="lg"
						iconRight={icons.code}
						class="rounded-fc-md"
					>
						GitHub
					</Button>
				</div>
			</section>

			<section class="bg-fc-accent text-fc-accent-fg">
				<div class="mx-auto max-w-fc-lg px-6 py-28 md:py-36">
					<div class="mb-10 flex items-center gap-3">
						<iconify-icon icon={icons.code} width="24" height="24" class="block"></iconify-icon>
						<span class="text-fc-xl font-semibold">Adapters</span>
					</div>
					<h2 class="max-w-2xl text-fc-3xl font-semibold leading-tight md:text-5xl">
						Écrivez une fois.<br />
						<span class="opacity-50">Déployez partout.</span>
					</h2>
					<WordReveal
						text="Vos règles et compétences sont écrites en markdown. Jardin génère automatiquement les fichiers de configuration pour chaque agent."
						dimColor={invertedDim}
						revealColor={invertedInk}
						class="mt-8 max-w-xl text-fc-md"
					/>

					<div class="mt-16 grid gap-4 sm:grid-cols-3">
						{#each adapters as adapter (adapter.file)}
							<div class="flex items-center gap-3 rounded-fc-md bg-fc-accent-fg/10 px-4 py-3">
								<iconify-icon icon={adapter.icon} width="20" height="20" class="block shrink-0"></iconify-icon>
								<div class="min-w-0">
									<p class="text-fc-sm font-medium">{adapter.agent}</p>
									<p class="mt-1 truncate font-fc-mono text-fc-xs opacity-60">{adapter.file}</p>
								</div>
							</div>
						{/each}
					</div>
				</div>
			</section>

			<section class="mx-auto max-w-fc-lg px-6 py-28 md:py-36">
				<div class="mb-16 max-w-lg">
					<h2 class="text-fc-3xl font-semibold leading-tight md:text-5xl">
						Trois couches.<br /><span class="text-fc-fg-muted">Chacune indépendante.</span>
					</h2>
					<p class="mt-4 text-fc-sm text-fc-fg-muted">
						Pas de dépendance. Chaque pièce fonctionne seule et s'enrichit avec les autres.
					</p>
				</div>

				<div class="grid gap-4 sm:grid-cols-3">
					{#each layers as layer (layer.title)}
						<Card class="flex flex-col gap-4">
							<span
								class="flex size-10 items-center justify-center rounded-fc-md bg-fc-surface text-fc-fg"
							>
								<iconify-icon icon={layer.icon} width="20" height="20" class="block"
								></iconify-icon>
							</span>
							<h3 class="text-fc-lg font-semibold text-fc-fg">{layer.title}</h3>
							<p class="text-fc-sm leading-relaxed text-fc-fg-muted">{layer.body}</p>
						</Card>
					{/each}
				</div>
			</section>

			<section class="bg-fc-surface">
				<div class="mx-auto max-w-fc-lg px-6 py-28 md:py-36">
					<div class="grid items-center gap-16 md:grid-cols-2">
						<div>
							<div class="mb-10 flex items-center gap-3">
								<iconify-icon icon={icons.usersGroup} width="24" height="24" class="block"
								></iconify-icon>
								<span class="text-fc-xl font-semibold text-fc-fg">Cells</span>
							</div>
							<h2 class="text-fc-3xl font-semibold leading-tight md:text-5xl">
								Perso vs. équipe.
							</h2>
							<WordReveal
								text="Un profil personnel, un profil équipe, un profil client — chaque cell a son propre memory, ses règles et ses skills. Superposez-les : les règles perso gagnent, la mémoire s'additionne."
								class="mt-8 max-w-lg text-fc-md"
							/>
						</div>
						<div class="grid gap-4">
							{#each cells as cell (cell.name)}
								<div
									class="rounded-fc-md px-5 py-4 {cell.active
										? 'bg-fc-accent text-fc-accent-fg'
										: 'bg-fc-component text-fc-fg'}"
								>
									<p class="font-fc-mono text-fc-sm font-medium">{cell.name}</p>
									<p
										class="mt-1 text-fc-xs {cell.active
											? 'text-fc-accent-fg/60'
											: 'text-fc-fg-muted'}"
									>
										{cell.desc}
									</p>
								</div>
							{/each}
						</div>
					</div>
				</div>
			</section>

			<section class="bg-fc-accent text-fc-accent-fg">
				<div class="mx-auto max-w-fc-lg px-6 py-28 md:py-36">
					<div class="mb-10 flex items-center gap-3">
						<iconify-icon icon={icons.refresh} width="24" height="24" class="block"
						></iconify-icon>
						<span class="text-fc-xl font-semibold">Sync</span>
					</div>
					<h2 class="text-fc-3xl font-semibold leading-tight md:text-5xl">
						Sync intégré.<br />
						<span class="opacity-50">Même binaire.</span>
					</h2>
					<WordReveal
						text="jardin serve lance un serveur de sync HTTP. Déployez-le sur votre VPS, connectez vos machines avec un token."
						dimColor={invertedDim}
						revealColor={invertedInk}
						class="mt-8 max-w-xl text-fc-md"
					/>

					<div class="mt-12 rounded-fc-md bg-fc-accent-fg/10 p-6 font-fc-mono text-fc-sm">
						<p class="opacity-50"># sur le serveur</p>
						<p>$ jardin serve --port 8420</p>
						<p class="mt-4 opacity-50"># sur votre machine</p>
						<p>$ jardin sync</p>
						<p class="mt-1 opacity-60">&nbsp; ↓ memory/tools/dokploy.md</p>
						<p class="opacity-60">&nbsp; ↑ rules/engineering-ladder.md</p>
						<p class="opacity-60">&nbsp; Synced 2 file(s).</p>
					</div>
				</div>
			</section>

			<section class="border-t border-fc-border">
				<div class="mx-auto max-w-fc-lg px-6 py-28 text-center md:py-36">
					<h2 class="text-fc-2xl font-semibold text-fc-fg">
						Open source. Local-first. Gratuit.
					</h2>
					<p class="mt-4 text-fc-sm text-fc-fg-muted">
						Un binaire Go. Zéro dépendance. Vos données restent chez vous.
					</p>
					<div class="mt-10 flex flex-wrap justify-center gap-3">
						<Button href="/login" size="lg" class="rounded-fc-md">Se connecter</Button>
						<Button
							href="https://github.com/FacileStudio/Jardin"
							target="_blank"
							rel="noopener noreferrer"
							variant="outline"
							size="lg"
							iconRight={icons.code}
							class="rounded-fc-md"
						>
							Voir le code
						</Button>
					</div>
				</div>
			</section>
		</main>

		<footer class="border-t border-fc-border">
			<div class="mx-auto max-w-fc-lg px-6 py-6 text-center text-fc-sm text-fc-fg-muted">
				© {new Date().getFullYear()} Jardin by
				<a href="https://facile.studio" class="text-fc-fg transition-opacity hover:opacity-70">
					Facile.
				</a>
			</div>
		</footer>
	</div>
{/if}
