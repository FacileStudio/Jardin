<script lang="ts">
	import { Alert, Input, SettingsRow, SettingsSection, Switch } from '@facile/muse';

	let {
		usageAlerts = $bindable(),
		usageThreshold = $bindable(),
		enabled,
		saving,
		onSettle
	}: {
		usageAlerts: boolean;
		usageThreshold: number | string;
		enabled: boolean;
		saving: boolean;
		onSettle: () => void;
	} = $props();
</script>

<SettingsSection
	title="Usage alerts"
	description="When a subscription window crosses your threshold, Jardin publishes one event to the Antenne. It fires once per window, not once per sync tick — the next alert waits for the window to reset. The Antenne owns what happens after that; Jardin sends nothing itself. The alert shows up in the Antenne's activity feed — anything that should act on it needs to subscribe to usage_alert.created on its side."
>
	{#if !enabled}
		<Alert tone="info" title="Emitting is off">
			Alerts ride the same socket as sessions. Turn emitting on above and they start flowing.
		</Alert>
	{/if}

	<SettingsRow
		label="Alert on usage"
		description="Off by default. Lowering the threshold later re-arms the current window."
	>
		<Switch bind:checked={usageAlerts} disabled={!enabled || saving} aria-label="Alert on usage" />
	</SettingsRow>

	<SettingsRow
		label="Threshold"
		description="Percent of a window's limit. 80 unless you change it; anything outside 1–100 falls back to it."
		for="bus-usage-threshold"
	>
		<Input
			bind:value={usageThreshold}
			id="bus-usage-threshold"
			type="number"
			inputmode="numeric"
			min="1"
			max="100"
			step="1"
			onblur={onSettle}
			disabled={!enabled || !usageAlerts || saving}
			class="w-24 tabular-nums"
		/>
	</SettingsRow>
</SettingsSection>
