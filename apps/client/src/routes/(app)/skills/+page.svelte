<script lang="ts">
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
			<p class="text-sm text-muted">Agent-agnostic skill definitions.</p>
		</div>
		<button onclick={addSkill} class="rounded-lg bg-primary px-3 py-1.5 text-sm font-medium text-primary-fg">
			Add skill
		</button>
	</div>

	<div class="flex gap-4">
		<div class="w-48 space-y-0.5">
			{#each skills as skill}
				<button
					onclick={() => selectSkill(skill)}
					class="w-full rounded-lg px-3 py-2 text-left text-sm transition-colors {selected === skill
						? 'bg-primary/10 font-medium text-primary'
						: 'text-muted hover:bg-surface'}"
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
						class="rounded-lg bg-primary px-3 py-1 text-sm text-primary-fg disabled:opacity-50"
					>
						{saving ? 'Saving...' : 'Save'}
					</button>
				</div>
				<textarea
					bind:value={content}
					class="h-96 w-full resize-none rounded-lg border border-border bg-bg p-3 font-mono text-sm outline-none focus:border-primary"
				></textarea>
			</div>
		{/if}
	</div>
</div>
