<script lang="ts">
	import { Button, Input, SettingsRow, SettingsSection, icons } from '@facile/muse';

	let {
		machineEmails = $bindable()
	}: {
		machineEmails: { machine: string; email: string }[];
	} = $props();
</script>

<SettingsSection
	title="Machine overrides"
	description="Sessions from a machine listed here are attributed to its own email instead of yours."
>
	{#each machineEmails as row, i (i)}
		<SettingsRow label="Override {i + 1}" stacked>
			<div class="flex w-full flex-col gap-2 sm:flex-row">
				<Input bind:value={row.machine} placeholder="machine" aria-label="Machine name" />
				<Input
					bind:value={row.email}
					type="email"
					placeholder="email"
					aria-label="Email for this machine"
					class="flex-1"
				/>
				<Button
					variant="ghost-danger"
					icon={icons.remove}
					aria-label="Remove override {i + 1}"
					onclick={() => (machineEmails = machineEmails.filter((_, j) => j !== i))}
				>
					Remove
				</Button>
			</div>
		</SettingsRow>
	{/each}

	<SettingsRow
		label="Add an override"
		description="One machine per row. Leave the name empty to drop a row on save."
	>
		<Button
			variant="outline"
			icon={icons.plus}
			onclick={() => (machineEmails = [...machineEmails, { machine: '', email: '' }])}
		>
			Add override
		</Button>
	</SettingsRow>
</SettingsSection>
