<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { ApiError, backend, TOKEN_KEY } from '$lib/backend';
	import {
		DEVICE_CODE_EXAMPLE,
		DEVICE_CODE_LENGTH,
		canonicaliseDeviceCode,
		deviceCodeDigits
	} from '$lib/deviceCode';

	/*
	 * What the page is doing, as one value.
	 *
	 * It used to be inferred from `machine` being a non-empty string, which collapsed three
	 * different situations into one: a code nobody has looked up yet, a lookup that failed,
	 * and a machine waiting to be approved all rendered the same entry form with no Authorize
	 * button anywhere. To somebody holding a code their terminal just printed, that reads
	 * exactly as "the approve button is disabled", and nothing on screen says otherwise.
	 */
	type Phase = 'starting' | 'entry' | 'looking' | 'missing' | 'ready' | 'approved' | 'denied';

	let phase = $state<Phase>('starting');
	let code = $state('');
	let machine = $state('');
	let ip = $state('');
	let busy = $state(false);
	let problem = $state('');
	let rejected = $state<string[]>([]);

	const digits = $derived(deviceCodeDigits(code));
	const complete = $derived(digits === DEVICE_CODE_LENGTH);
	const hint = $derived(entryHint(digits, rejected));

	onMount(async () => {
		const url = new URL(window.location.href);
		if (!localStorage.getItem(TOKEN_KEY)) {
			goto('/login?redirect=' + encodeURIComponent('/authorize' + url.search));
			return;
		}
		const linked = canonicaliseDeviceCode(url.searchParams.get('code') ?? '');
		code = linked.code;
		if (linked.digits === DEVICE_CODE_LENGTH) {
			await lookUp();
			return;
		}
		phase = 'entry';
	});

	/*
	 * Canonicalising here covers paste as well as typing. A paste raises an input event with
	 * inputType "insertFromPaste", where the paste event itself fires before the field holds
	 * the new text, so a handler on paste would normalise the previous value.
	 *
	 * Writing the canonical string back to the field is what keeps what the user sees equal to
	 * what gets sent, and it leaves the caret at the end of a short code, which is where the
	 * next character goes anyway. The caret snapping back to the start on every keystroke is
	 * the version of this that would be unusable.
	 */
	function onCodeInput(event: Event & { currentTarget: HTMLInputElement }) {
		const entry = canonicaliseDeviceCode(event.currentTarget.value);
		code = entry.code;
		rejected = entry.rejected;
		event.currentTarget.value = entry.code;
		problem = '';
		phase = 'entry';
	}

	/*
	 * What to say under the field. Dropped characters win over everything else: a keystroke
	 * that put nothing on screen needs explaining before progress does.
	 */
	function entryHint(count: number, dropped: string[]): string {
		if (dropped.length > 0) {
			const list = dropped.join(', ');
			return `${list} ${dropped.length === 1 ? 'is' : 'are'} not used in device codes, so nothing was added.`;
		}
		if (count === 0) return `Eight characters, like ${DEVICE_CODE_EXAMPLE}.`;
		return `${count} of ${DEVICE_CODE_LENGTH} characters.`;
	}

	/*
	 * Resolve the code into the machine it belongs to. Every ending has its own phase and its
	 * own sentence: found and waiting, found but already settled, or not held by this server
	 * at all. None of them leaves the page in a state whose only signal is a missing button.
	 *
	 * The code is captured before the request so a reply that arrives after the user has
	 * carried on typing is dropped rather than shown. Approving the wrong machine because a
	 * slow lookup won a race is not a failure mode worth having.
	 */
	async function lookUp() {
		const asked = code;
		phase = 'looking';
		problem = '';
		rejected = [];
		try {
			const info = await backend.deviceInfo(asked);
			if (asked !== code) return;
			machine = info.machine;
			ip = info.ip;
			if (info.status !== 'pending') {
				phase = 'missing';
				problem = `This code was already ${info.status}. Run mycelium login again for a fresh one.`;
				return;
			}
			phase = 'ready';
		} catch (err) {
			if (asked !== code) return;
			if (err instanceof ApiError && err.status === 401) {
				goto('/login?redirect=' + encodeURIComponent('/authorize?code=' + asked));
				return;
			}
			machine = '';
			phase = 'missing';
			problem = refusal(err);
		}
	}

	/*
	 * Why the server said no, in words the person can act on. The status is the whole point:
	 * an account that may not approve machines and a code this server has never held both
	 * used to render the same "not found", which sends somebody hunting for a typo in a code
	 * that was never wrong.
	 *
	 * The restart is named because it is the likeliest cause. Pending device codes live in
	 * process memory (internal/server/device.go), so every deploy of the API drops every code
	 * issued in the ten minutes before it, and the code in the terminal is still on screen
	 * looking perfectly valid.
	 */
	function refusal(err: unknown): string {
		if (err instanceof ApiError && err.status === 403) {
			return 'Approving a machine needs an admin account, and this session is not one. Ask an admin of this Mycelium to approve it.';
		}
		return 'This server is not holding that code. It may have expired, been used already, or been issued before the server last restarted. Run mycelium login again for a fresh one.';
	}

	/*
	 * Continue never refuses in silence. An unfinished code is answered in words rather than
	 * by greying the button out, because a button that does nothing and explains nothing is
	 * the complaint this page exists to answer.
	 */
	function submit(event: SubmitEvent) {
		event.preventDefault();
		if (!complete) {
			problem = `That code is not finished. Device codes are ${DEVICE_CODE_LENGTH} characters, like ${DEVICE_CODE_EXAMPLE}.`;
			return;
		}
		lookUp();
	}

	/*
	 * A refused approval leaves the message on screen and the buttons where they are, so a
	 * transient failure can simply be pressed again. A 404 is the other kind: the request is
	 * gone from the server and there is nothing left to approve, so the page says so rather
	 * than offering a button that can only fail.
	 */
	function settleFailed(err: unknown) {
		problem = refusal(err);
		if (err instanceof ApiError && err.status === 404) phase = 'missing';
	}

	async function approve() {
		busy = true;
		problem = '';
		try {
			await backend.deviceApprove(code);
			phase = 'approved';
		} catch (err) {
			settleFailed(err);
		} finally {
			busy = false;
		}
	}

	async function deny() {
		busy = true;
		problem = '';
		try {
			await backend.deviceDeny(code);
			phase = 'denied';
		} catch (err) {
			settleFailed(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head>
	<title>Authorize a machine — Mycelium</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-background px-6 py-12">
	<div class="w-full max-w-sm">
		<a href="/memory" class="mb-8 flex items-center justify-center gap-2.5">
			<iconify-icon icon="solar:structure-bold-duotone" width="28" height="28" class="block text-foreground"></iconify-icon>
			<span class="text-xl font-bold tracking-tight">Mycelium</span>
		</a>

		{#if phase === 'starting'}
			<div class="h-40"></div>
		{:else if phase === 'approved'}
			<div class="rounded-xl border border-border bg-card p-6 text-center">
				<iconify-icon icon="solar:check-circle-bold" width="40" height="40" class="mx-auto block text-fc-success"></iconify-icon>
				<h1 class="mt-3 text-lg font-semibold">Machine authorized</h1>
				<p class="mt-1 text-sm text-muted-foreground">
					<span class="font-medium text-foreground">{machine}</span> can now sync. Return to your
					terminal — it will finish automatically.
				</p>
			</div>
		{:else if phase === 'denied'}
			<div class="rounded-xl border border-border bg-card p-6 text-center">
				<iconify-icon icon="solar:close-circle-bold" width="40" height="40" class="mx-auto block text-muted-foreground"></iconify-icon>
				<h1 class="mt-3 text-lg font-semibold">Request denied</h1>
				<p class="mt-1 text-sm text-muted-foreground">No token was issued.</p>
			</div>
		{:else}
			<div class="mb-6 text-center">
				<h1 class="text-xl font-bold tracking-tight">Authorize a machine</h1>
				<p class="mt-1.5 text-sm text-muted-foreground">
					Confirm the code shown in your terminal to let this machine sync.
				</p>
			</div>

			{#if phase === 'ready'}
				<div class="rounded-xl border border-border bg-card p-5">
					<div class="flex items-center gap-3">
						<div class="flex size-10 items-center justify-center rounded-lg bg-accent">
							<iconify-icon icon="solar:server-square-linear" width="20" height="20" class="block"></iconify-icon>
						</div>
						<div>
							<p class="text-sm text-muted-foreground">Machine requesting access</p>
							<p class="font-semibold">{machine}</p>
						</div>
					</div>
					<div class="mt-3 space-y-1 font-mono text-sm text-muted-foreground">
						<p>code <span class="text-foreground">{code}</span></p>
						{#if ip}<p>from <span class="text-foreground">{ip}</span></p>{/if}
					</div>

					<p class="mt-4 rounded-md bg-fc-warning/10 px-3 py-2 text-xs text-fc-warning">
						Only authorize if you just ran <code>mycelium login</code> on this machine. Approving grants
						it ongoing sync access to your shared brain.
					</p>

					<div class="mt-5 flex gap-2">
						<button
							type="button"
							onclick={deny}
							disabled={busy}
							class="inline-flex h-10 flex-1 items-center justify-center rounded-md border border-border bg-background px-4 text-sm font-medium hover:bg-accent disabled:opacity-50"
						>
							Deny
						</button>
						<button
							type="button"
							onclick={approve}
							disabled={busy}
							class="inline-flex h-10 flex-1 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
						>
							{busy ? 'Authorizing…' : 'Authorize'}
						</button>
					</div>
				</div>
			{:else}
				<form onsubmit={submit} class="space-y-3">
					<div class="space-y-1.5">
						<input
							value={code}
							oninput={onCodeInput}
							placeholder={DEVICE_CODE_EXAMPLE}
							autocomplete="off"
							autocapitalize="characters"
							spellcheck="false"
							aria-label="Device code"
							aria-describedby="code-hint"
							class="h-11 w-full rounded-md border border-input bg-background px-3 text-center font-mono text-lg uppercase tracking-widest outline-none focus-visible:ring-2 focus-visible:ring-ring"
						/>
						<p id="code-hint" class="text-center text-xs text-muted-foreground">{hint}</p>
					</div>
					<button
						type="submit"
						disabled={phase === 'looking'}
						class="inline-flex h-10 w-full items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
					>
						{phase === 'looking' ? 'Checking…' : 'Continue'}
					</button>
				</form>
			{/if}

			{#if problem}
				<p class="mt-4 text-center text-sm text-destructive">{problem}</p>
			{/if}
		{/if}
	</div>
</div>
