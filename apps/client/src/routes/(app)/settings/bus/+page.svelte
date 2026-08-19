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
	import { ApiError, backend, type AntenneSettings, type EmitterStatus } from '$lib/backend';
	import BusMachineOverrides from '$lib/components/BusMachineOverrides.svelte';
	import BusUsageAlerts from '$lib/components/BusUsageAlerts.svelte';

	let loaded = $state(false);
	let denied = $state(false);
	let saving = $state(false);
	let error = $state('');
	let loadError = $state('');

	let enabled = $state(false);
	let instance = $state('');
	let secret = $state('');
	let userEmail = $state('');
	let emitSince = $state('');
	let usageAlerts = $state(false);
	let usageThreshold: number | string = $state(80);
	let machineEmails: { machine: string; email: string }[] = $state([]);
	let status: EmitterStatus | null = $state(null);
	let envManaged = $state<Record<string, boolean>>({});

	$effect(() => {
		backend
			.settingsGet()
			.then((s) => {
				apply(s.antenne, s.status);
				envManaged = s.env_managed ?? {};
				loaded = true;
			})
			.catch((e) => {
				// Only a 403 means "not an admin". Everything else — the API
				// restarting mid-deploy, a dropped connection, a 500 — used to
				// land here too and get reported as a permission problem,
				// which sends you looking at your account instead of at the
				// server. Say what actually happened.
				if (e instanceof ApiError && e.status === 403) {
					denied = true;
					return;
				}
				// A failed load is not a failed save. They were one variable, so
				// opening the page on a broken response greeted you with "Not
				// saved" for something you never submitted.
				loadError = e instanceof Error ? e.message : 'Could not load the bus settings';
			});
	});

	function apply(antenne: AntenneSettings, next: EmitterStatus) {
		enabled = antenne.enabled;
		instance = antenne.instance;
		secret = antenne.secret;
		userEmail = antenne.user_email;
		emitSince = antenne.emit_since ?? '';
		usageAlerts = antenne.usage_alerts ?? false;
		usageThreshold = normalizeThreshold(antenne.usage_threshold);
		machineEmails = Object.entries(antenne.machine_emails ?? {}).map(([machine, email]) => ({
			machine,
			email
		}));
		status = next;
	}

	const DEFAULT_THRESHOLD = 80;

	function normalizeThreshold(raw: number | string | undefined): number {
		const n = Math.round(Number(raw));
		if (!Number.isFinite(n) || n <= 0) return DEFAULT_THRESHOLD;
		return Math.min(100, n);
	}

	function settleThreshold() {
		usageThreshold = normalizeThreshold(usageThreshold);
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
				emit_since: emitSince || undefined,
				usage_alerts: usageAlerts,
				usage_threshold: normalizeThreshold(usageThreshold)
			});
			apply(s.antenne, s.status);
			envManaged = s.env_managed ?? {};
			toast.success('Bus settings saved.');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Save failed';
		} finally {
			saving = false;
		}
	}
</script>

{#if denied}
	<Alert tone="info" title="Admin only">
		Bus settings are visible to administrators. Sign in with an admin account to change them.
	</Alert>
{:else if loadError}
	<Alert tone="danger" title="Could not load the bus settings">{loadError}</Alert>
{:else if !loaded}
	<div class="flex items-center gap-3 text-fc-sm text-fc-fg-muted">
		<Spinner size="sm" /> Loading…
	</div>
{:else}
	<div class="flex flex-col gap-10">
		<SettingsSection
			title="Antenne bus"
			description="One socket to Antenne carries every sealed session this instance emits, so Sablier can turn them into time entries. Apps never talk to each other directly."
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

			{#if usageAlerts}
				<SettingsRow
					label="Alert outbox"
					description="Usage alerts held locally until the socket comes back."
				>
					<Badge tone={(status?.usage_alerts_pending ?? 0) > 0 ? 'accent' : 'neutral'}>
						{status?.usage_alerts_pending ?? 0} pending
					</Badge>
				</SettingsRow>
			{/if}

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

		<BusUsageAlerts
			bind:usageAlerts
			bind:usageThreshold
			{enabled}
			{saving}
			onSettle={settleThreshold}
		/>

		<SettingsSection
			title="Connection"
			description="Where Antenne lives and the shared secret that registers this instance with it."
		>
			<SettingsRow
				label="Instance URL"
				description={envManaged.instance
					? 'Pinned by ANTENNE_URL. Edit the deployment, not this field.'
					: 'Scheme included. The socket URL is derived from it.'}
				for="bus-url"
				stacked
			>
				<Input
					bind:value={instance}
					id="bus-url"
					placeholder="https://antenne.facile.studio"
					disabled={saving || envManaged.instance}
				/>
			</SettingsRow>

			<SettingsRow
				label="Shared secret"
				description={envManaged.secret
					? 'Pinned by ANTENNE_SECRET. Edit the deployment, not this field.'
					: 'Posted to Antenne when the socket registers. Required as soon as emitting is on.'}
				stacked
			>
				<SecretField
					bind:value={secret}
					editable={!envManaged.secret}
					disabled={saving || envManaged.secret}
					class="w-full"
				/>
			</SettingsRow>

			<SettingsRow
				label="Attribution email"
				description={envManaged.user_email
					? 'Pinned by ANTENNE_USER_EMAIL. Edit the deployment, not this field.'
					: 'Whose sessions these are, as far as the rest of the suite is concerned.'}
				for="bus-email"
				stacked
			>
				<Input
					bind:value={userEmail}
					id="bus-email"
					type="email"
					placeholder="you@facile.studio"
					disabled={saving || envManaged.user_email}
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

		<BusMachineOverrides bind:machineEmails />
	</div>
{/if}
