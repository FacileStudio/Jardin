<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { icons, toast } from '@facile/muse';
	import { backend } from '$lib/backend';
	import DocumentEditor from '$lib/components/DocumentEditor.svelte';

	const rel = $derived(page.params.path ?? '');
	const fullPath = $derived(`memory/${rel}`);
	const title = $derived(rel.split('/').pop()?.replace(/\.md$/, '') ?? rel);
	let content = $state('');
	let loading = $state(true);

	$effect(() => {
		const p = fullPath;
		loading = true;
		backend
			.syncFile(p)
			.then((c) => (content = c))
			.catch((e) => {
				content = '';
				toast.danger(e instanceof Error ? e.message : `Could not load ${p}.`);
			})
			.finally(() => (loading = false));
	});

	async function save(next: string) {
		await backend.syncFilePut(fullPath, next);
		content = next;
	}

	async function remove() {
		await backend.syncFileDelete(fullPath);
		toast.success(`Deleted ${fullPath}.`);
		goto('/memory');
	}
</script>

<DocumentEditor
	{title}
	path={fullPath}
	icon={icons.folder}
	backHref="/memory"
	backLabel="Memory"
	{content}
	{loading}
	deleteTitle="Delete “{title}”?"
	deleteDescription="The page is removed from every machine on the next sync, and nothing keeps a copy. It cannot be undone."
	onSave={save}
	onDelete={remove}
/>
