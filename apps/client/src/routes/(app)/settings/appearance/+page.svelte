<script lang="ts">
	import { OptionCards, SettingsRow, SettingsSection, icons } from '@facile/muse';
	import { setTheme, theme, type ThemeMode } from '$lib/theme.svelte';

	const modes = [
		{ value: 'system', label: 'System', icon: icons.monitor },
		{ value: 'light', label: 'Light', icon: icons.sun },
		{ value: 'dark', label: 'Dark', icon: icons.moon }
	];

	let mode = $state<string>(theme.mode);

	$effect(() => {
		setTheme(mode as ThemeMode);
	});
</script>

<div class="flex flex-col gap-10">
	<SettingsSection
		title="Theme"
		description="Applied to this browser. Every device you sign in from keeps its own choice."
	>
		<SettingsRow
			label="Colour scheme"
			description="System follows your OS for as long as the app is open."
			stacked
		>
			<OptionCards options={modes} bind:value={mode} name="theme-mode" label="Colour scheme" />
		</SettingsRow>
	</SettingsSection>
</div>
