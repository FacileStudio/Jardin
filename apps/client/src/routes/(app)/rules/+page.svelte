<script lang="ts">
	import Icon from '@iconify/svelte';
	import { goto } from '$app/navigation';
	import { backend } from '$lib/backend';

	let rules: string[] = $state([]);

	$effect(() => {
		backend.rulesList().then((r) => (rules = r)).catch(() => {});
	});

	async function addRule() {
		const name = prompt('Rule name (prefix to order, e.g. 30-testing):');
		if (!name) return;
		await backend.ruleSave(name, `# ${name}\n`);
		goto(`/rules/${name}`);
	}
</script>

<div class="space-y-7">
	<div class="flex items-end justify-between gap-4">
		<div>
			<h2 class="text-2xl font-semibold tracking-tight">Rules</h2>
			<p class="mt-1 text-sm text-muted-foreground">Modular instructions, concatenated into every agent config in filename order.</p>
		</div>
		<button onclick={addRule} class="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-primary px-3.5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90">
			<Icon icon="solar:add-circle-linear" class="size-4" />
			New rule
		</button>
	</div>

	{#if rules.length === 0}
		<div class="rounded-xl border border-dashed border-border p-12 text-center">
			<Icon icon="solar:ruler-angular-linear" class="mx-auto size-6 text-muted-foreground/50" />
			<p class="mt-2 text-sm text-muted-foreground">No rules yet. Create one to shape your agents.</p>
		</div>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each rules as rule}
				<a
					href="/rules/{rule}"
					class="group rounded-xl border border-border bg-background p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-sm"
				>
					<div class="flex items-center justify-between">
						<div class="flex size-9 items-center justify-center rounded-lg bg-accent">
							<Icon icon="solar:ruler-angular-linear" class="size-[18px] text-foreground" />
						</div>
						<Icon icon="solar:alt-arrow-right-linear" class="size-4 text-muted-foreground/30 transition-all group-hover:translate-x-0.5 group-hover:text-muted-foreground" />
					</div>
					<p class="mt-3 truncate font-medium">{rule}</p>
					<p class="font-mono text-xs text-muted-foreground">rules/{rule}.md</p>
				</a>
			{/each}
		</div>
	{/if}
</div>
