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
			.skillGet(n)
			.then((c) => (content = c))
			.catch((e) => {
				content = '';
				toast.danger(e instanceof Error ? e.message : `Could not load skill “${n}”.`);
			})
			.finally(() => (loading = false));
	});

	async function save(next: string) {
		await backend.skillSave(name, next);
		content = next;
	}

	async function remove() {
		await backend.skillDelete(name);
		toast.success(`Skill “${name}” deleted.`);
		goto('/skills');
	}
</script>

<DocumentEditor
	title={name}
	path="skills/{name}.md"
	icon={icons.bolt}
	backHref="/skills"
	backLabel="Skills"
	{content}
	{loading}
	deleteTitle="Delete “{name}”?"
	deleteDescription="The skill disappears from every agent on the next sync. It cannot be undone."
	onSave={save}
	onDelete={remove}
/>
