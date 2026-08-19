<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { icons, toast } from '@facile/muse';
	import { backend } from '$lib/backend';
	import DocumentEditor from '$lib/components/DocumentEditor.svelte';

	const name = $derived(page.params.name ?? '');
	let content = $state('');
	let loading = $state(true);

	$effect(() => {
		const n = name;
		loading = true;
		backend
			.ruleGet(n)
			.then((c) => (content = c))
			.catch((e) => {
				content = '';
				toast.danger(e instanceof Error ? e.message : `Could not load rule “${n}”.`);
			})
			.finally(() => (loading = false));
	});

	async function save(next: string) {
		await backend.ruleSave(name, next);
		content = next;
	}

	async function remove() {
		await backend.ruleDelete(name);
		toast.success(`Rule “${name}” deleted.`);
		goto('/rules');
	}
</script>

<DocumentEditor
	title={name}
	path="rules/{name}.md"
	icon={icons.shield}
	backHref="/rules"
	backLabel="Rules"
	{content}
	{loading}
	deleteTitle="Delete “{name}”?"
	deleteDescription="Every agent config is rebuilt without this rule on the next sync. It cannot be undone."
	onSave={save}
	onDelete={remove}
/>
