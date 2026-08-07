<script lang="ts">
	import { getContext } from 'svelte';
	import { SecretField, SettingsRow, SettingsSection, StatusDot } from '@facile/muse';
	import { backend, type AuthConfig, type JardinStatus } from '$lib/backend';

	const status = getContext<() => JardinStatus | null>('status');

	let config: AuthConfig | null = $state(null);

	$effect(() => {
		backend
			.getAuthConfig()
			.then((c) => (config = c))
			.catch(() => (config = null));
	});

	const authMode = $derived.by(() => {
		if (!config) return { tone: 'neutral' as const, label: 'Unknown' };
		if (config.sso_only) return { tone: 'success' as const, label: 'SSO only' };
		if (config.oidc_enabled)
			return { tone: 'info' as const, label: 'SSO and shared password' };
		return { tone: 'warning' as const, label: 'Shared password only' };
	});
</script>

<div class="flex flex-col gap-10">
	<SettingsSection
		title="Instance"
		description="Useful when you file a bug against this self-hosted install."
	>
		<SettingsRow label="Sign-in" description="How this instance authenticates people.">
			<StatusDot tone={authMode.tone} label={authMode.label} />
		</SettingsRow>

		<SettingsRow label="This machine" description="The name the server reports for itself." stacked>
			<SecretField value={status?.()?.machine ?? '—'} sensitive={false} class="w-full" />
		</SettingsRow>

		<SettingsRow label="Rules loaded" description="Concatenated into every agent config.">
			<span class="text-fc-sm tabular-nums text-fc-fg">{status?.()?.rules?.length ?? 0}</span>
		</SettingsRow>

		<SettingsRow label="Skills loaded" description="Installed into each agent's skill format.">
			<span class="text-fc-sm tabular-nums text-fc-fg">{status?.()?.skills?.length ?? 0}</span>
		</SettingsRow>
	</SettingsSection>

	<SettingsSection
		title="Command line"
		description="The CLI is the same binary as the server, so a machine only needs the install script and a token."
	>
		<SettingsRow label="Install" stacked>
			<SecretField
				value="curl -fsSL https://raw.githubusercontent.com/FacileStudio/Jardin/main/install.sh | bash"
				sensitive={false}
				class="w-full"
			/>
		</SettingsRow>

		<SettingsRow label="Pair this machine" description="Paste an API token when it asks." stacked>
			<SecretField
				value="jardin login https://jardin.facile.studio"
				sensitive={false}
				class="w-full"
			/>
		</SettingsRow>
	</SettingsSection>
</div>
