<script lang="ts">
	import Icon from '@iconify/svelte';
	import { backend } from '$lib/backend';

	let skills: string[] = $state([]);
	let selected = $state('');
	let content = $state('');
	let saving = $state(false);

	$effect(() => {
		backend.skillsList().then((s) => (skills = s)).catch(() => {});
	});

	async function selectSkill(name: string) {
		selected = name;
		content = await backend.skillGet(name);
	}

	async function save() {
		if (!selected) return;
		saving = true;
		try {
			await backend.skillSave(selected, content);
		} finally {
			saving = false;
		}
	}

	async function addSkill() {
		const name = prompt('Skill name:');
		if (!name) return;
		const template = `---\nname: ${name}\ndescription: ""\ntriggers: ["/${name}"]\n---\n\n# ${name}\n`;
		await backend.skillSave(name, template);
		skills = await backend.skillsList();
		selectSkill(name);
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-semibold">Skills</h2>
			<p class="text-sm text-muted-foreground">Agent-agnostic skill definitions.</p>
		</div>
		<button onclick={addSkill} class="inline-flex items-center gap-1.5 rounded-md border border-border bg-background px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground">
			<Icon icon="solar:add-circle-linear" class="size-4" />
			Add skill
		</button>
	</div>

	<div class="flex gap-4">
		<div class="w-48 space-y-0.5">
			{#each skills as skill}
				<button
					onclick={() => selectSkill(skill)}
					class="w-full rounded-lg px-3 py-2 text-left text-sm transition-colors {selected === skill
						? 'bg-accent font-medium text-foreground'
						: 'text-muted-foreground hover:bg-accent'}"
				>
					{skill}
				</button>
			{/each}
		</div>

		{#if selected}
			<div class="flex-1 space-y-3">
				<div class="flex items-center justify-between">
					<h3 class="text-sm font-medium">{selected}.md</h3>
					<button
						onclick={save}
						disabled={saving}
						class="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
					>
						<Icon icon="solar:diskette-linear" class="size-3.5" />
						{saving ? 'Saving...' : 'Save'}
					</button>
				</div>
				<textarea
					bind:value={content}
					class="h-96 w-full resize-none rounded-md border border-input bg-background p-3 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
				></textarea>
			</div>
		{/if}
	</div>
</div>
