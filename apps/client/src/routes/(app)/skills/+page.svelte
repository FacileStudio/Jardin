<script lang="ts">
	import Icon from '@iconify/svelte';
	import { goto } from '$app/navigation';
	import { backend } from '$lib/backend';

	let skills: string[] = $state([]);

	$effect(() => {
		backend.skillsList().then((s) => (skills = s)).catch(() => {});
	});

	async function addSkill() {
		const name = prompt('Skill name:');
		if (!name) return;
		const template = `---\nname: ${name}\ndescription: ""\ntriggers: ["/${name}"]\n---\n\n# ${name}\n`;
		await backend.skillSave(name, template);
		goto(`/skills/${name}`);
	}
</script>

<div class="space-y-7">
	<div class="flex items-end justify-between gap-4">
		<div>
			<h2 class="text-2xl font-semibold tracking-tight">Skills</h2>
			<p class="mt-1 text-sm text-muted-foreground">Agent-agnostic capabilities, installed into each agent's skill format.</p>
		</div>
		<button onclick={addSkill} class="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-primary px-3.5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90">
			<Icon icon="solar:add-circle-linear" class="size-4" />
			New skill
		</button>
	</div>

	{#if skills.length === 0}
		<div class="rounded-xl border border-dashed border-border p-12 text-center">
			<Icon icon="solar:bolt-circle-linear" class="mx-auto size-6 text-muted-foreground/50" />
			<p class="mt-2 text-sm text-muted-foreground">No skills yet. Add one to teach every agent a new trick.</p>
		</div>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each skills as skill}
				<a
					href="/skills/{skill}"
					class="group rounded-xl border border-border bg-background p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-sm"
				>
					<div class="flex items-center justify-between">
						<div class="flex size-9 items-center justify-center rounded-lg bg-accent">
							<Icon icon="solar:bolt-circle-linear" class="size-[18px] text-foreground" />
						</div>
						<Icon icon="solar:alt-arrow-right-linear" class="size-4 text-muted-foreground/30 transition-all group-hover:translate-x-0.5 group-hover:text-muted-foreground" />
					</div>
					<p class="mt-3 truncate font-medium">{skill}</p>
					<p class="font-mono text-xs text-muted-foreground">skills/{skill}.md</p>
				</a>
			{/each}
		</div>
	{/if}
</div>
