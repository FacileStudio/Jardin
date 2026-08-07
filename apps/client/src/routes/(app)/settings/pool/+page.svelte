<script lang="ts">
	import {
		Alert,
		Badge,
		Button,
		Input,
		SecretField,
		SettingsRow,
		SettingsSection,
		Spinner,
		StatusDot,
		Switch,
		icons,
		toast
	} from '@facile/muse';
	import { backend, type EmitterStatus, type NookSettings } from '$lib/backend';

	let loaded = $state(false);
	let denied = $state(false);
	let saving = $state(false);
	let error = $state('');

	let enabled = $state(false);
	let instance = $state('');
	let secret = $state('');
	let userEmail = $state('');
	let emitSince = $state('');
	let machineEmails: { machine: string; email: string }[] = $state([]);
	let status: EmitterStatus | null = $state(null);

	$effect(() => {
		backend
			.settingsGet()
			.then((s) => {
				apply(s.nook, s.status);
				loaded = true;
			})
			.catch(() => (denied = true));
	});

	function apply(nook: NookSettings, next: EmitterStatus) {
		enabled = nook.enabled;
		instance = nook.instance;
		secret = nook.secret;
		userEmail = nook.user_email;
		emitSince = nook.emit_since ?? '';
		machineEmails = Object.entries(nook.machine_emails ?? {}).map(([machine, email]) => ({
			machine,
			email
		}));
		status = next;
	}

	/*
	 * Four states, not a boolean. "Not connected" hides a switch that is off, a socket still
	 * opening and a handshake that failed — three situations with three different fixes.
	 */
	const connection = $derived.by(
		(): { tone: 'success' | 'warning' | 'danger' | 'neutral'; label: string; pulse: boolean } => {
			if (!enabled) return { tone: 'neutral', label: 'Disabled', pulse: false };
			if (status?.connected) return { tone: 'success', label: 'Connected', pulse: false };
			if (status?.last_error)
				return { tone: 'danger', label: `Disconnected — ${status.last_error}`, pulse: false };
			return { tone: 'warning', label: 'Connecting…', pulse: true };
		}
	);

	async function save(event?: Event) {
		event?.preventDefault();
		saving = true;
		error = '';
		try {
			const emails: Record<string, string> = {};
			for (const row of machineEmails) {
				if (row.machine.trim()) emails[row.machine.trim()] = row.email.trim();
			}
			const s = await backend.settingsSave({
				enabled,
				instance,
				secret,
				user_email: userEmail,
				machine_emails: emails,
				emit_since: emitSince || undefined
			});
			apply(s.nook, s.status);
			toast.success('Pool settings saved.');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Save failed';
		} finally {
			saving = false;
		}
	}
</script>

{#if denied}
	<Alert tone="info" title="Admin only">
		Pool settings are visible to administrators. Sign in with an admin account to change them.
	</Alert>
{:else if !loaded}
	<div class="flex items-center gap-3 text-fc-sm text-fc-fg-muted">
		<Spinner size="sm" /> Loading…
	</div>
{:else}
	<div class="flex flex-col gap-10">
		<SettingsSection
			title="Nook Pool"
			description="One socket to Nook carries every sealed session this instance emits, so Sablier can turn them into time entries. Apps never talk to each other directly."
		>
			<SettingsRow label="Status" description="Live, from the socket itself — not from the last save.">
				<StatusDot tone={connection.tone} label={connection.label} pulse={connection.pulse} />
			</SettingsRow>

			<SettingsRow label="Emit sessions" description="Off keeps the config but stops the socket.">
				<Switch bind:checked={enabled} aria-label="Emit sessions" />
			</SettingsRow>

			<SettingsRow label="Emitted" description="Sessions this instance has published since it started.">
				<Badge tone="neutral">{status?.emitted ?? 0} sent</Badge>
			</SettingsRow>

			<SettingsRow label="Outbox" description="Sealed sessions held locally until the socket comes back.">
				<Badge tone={(status?.pending ?? 0) > 0 ? 'accent' : 'neutral'}>
					{status?.pending ?? 0} pending
				</Badge>
			</SettingsRow>

			{#if emitSince}
				<SettingsRow
					label="Emitting since"
					description="Sessions that ended before this are never republished."
					stacked
				>
					<SecretField value={emitSince} sensitive={false} class="w-full" />
				</SettingsRow>
			{/if}
		</SettingsSection>

		<SettingsSection
			title="Connection"
			description="Where Nook lives and the shared secret that registers this instance with it."
		>
			<SettingsRow
				label="Instance URL"
				description="Scheme included. The socket URL is derived from it."
				for="pool-url"
				stacked
			>
				<Input
					bind:value={instance}
					id="pool-url"
					placeholder="https://nook.facile.studio"
					disabled={saving}
				/>
			</SettingsRow>

			<SettingsRow
				label="Shared secret"
				description="Posted to Nook when the socket registers. Required as soon as emitting is on."
				stacked
			>
				<SecretField bind:value={secret} editable disabled={saving} class="w-full" />
			</SettingsRow>

			<SettingsRow
				label="Attribution email"
				description="Whose sessions these are, as far as the rest of the suite is concerned."
				for="pool-email"
				stacked
			>
				<Input
					bind:value={userEmail}
					id="pool-email"
					type="email"
					placeholder="you@facile.studio"
					disabled={saving}
				/>
			</SettingsRow>

			{#if error}
				<Alert tone="danger" title="Not saved">{error}</Alert>
			{/if}

			<div class="flex flex-wrap gap-2 pt-2">
				<Button icon={icons.plug} disabled={saving} onclick={save}>
					{saving ? 'Saving…' : 'Save and connect'}
				</Button>
			</div>
		</SettingsSection>

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
	</div>
{/if}
