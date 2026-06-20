<script lang="ts">
	import Icon from '@iconify/svelte';
	import { backend } from '$lib/backend';

	let rules: string[] = $state([]);
	let selected = $state('');
	let content = $state('');
	let saving = $state(false);

	$effect(() => {
		backend.rulesList().then((r) => (rules = r)).catch(() => {});
	});

	async function selectRule(name: string) {
		selected = name;
		content = await backend.ruleGet(name);
	}

	async function save() {
		if (!selected) return;
		saving = true;
		try {
			await backend.ruleSave(selected, content);
		} finally {
			saving = false;
		}
	}

	async function addRule() {
		const name = prompt('Rule name:');
		if (!name) return;
		await backend.ruleSave(name, `# ${name}\n`);
		rules = await backend.rulesList();
		selectRule(name);
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-semibold">Rules</h2>
			<p class="text-sm text-muted-foreground">Agent instructions concatenated into configs.</p>
		</div>
		<button onclick={addRule} class="inline-flex items-center gap-1.5 rounded-md border border-border bg-background px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground">
			<Icon icon="solar:add-circle-linear" class="size-4" />
			Add rule
		</button>
	</div>

	<div class="flex gap-4">
		<div class="w-48 space-y-0.5">
			{#each rules as rule}
				<button
					onclick={() => selectRule(rule)}
					class="w-full rounded-lg px-3 py-2 text-left text-sm transition-colors {selected === rule
						? 'bg-accent font-medium text-foreground'
						: 'text-muted-foreground hover:bg-accent'}"
				>
					{rule}
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
