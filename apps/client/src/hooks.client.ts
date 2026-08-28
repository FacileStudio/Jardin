import { dev, version } from '$app/environment';
import { createDeferredJournal } from '@facile/journal';
import { handleErrorWith } from '@facile/journal/sveltekit';
import { backend } from '$lib/backend';

const journal = createDeferredJournal(async () => {
	const config = await backend.getAuthConfig();
	if (!config.journal?.url || !config.journal?.key) return null;
	return {
		url: config.journal.url,
		key: config.journal.key,
		release: version,
		environment: dev ? 'development' : 'production'
	};
});

journal.install();

export const handleError = handleErrorWith(journal);
