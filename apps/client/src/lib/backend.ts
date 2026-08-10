import { getActiveSpaceId } from './space.svelte';

const BASE = '/api';

export const TOKEN_KEY = 'jardin.token';

function spaceQuery(sep: '?' | '&' = '?'): string {
	const id = getActiveSpaceId();
	return id ? `${sep}space_id=${encodeURIComponent(id)}` : '';
}

function getToken(): string | null {
	if (typeof window === 'undefined') return null;
	return localStorage.getItem(TOKEN_KEY);
}

type ApiErrorPayload = {
	error?: { message?: string };
};

/**
 * ApiError carries the HTTP status alongside the message.
 *
 * Without it a caller can only see a string, so every failure looks alike —
 * which is how the pool settings page came to answer "you are not an admin"
 * to a server that was simply restarting. A page that wants to special-case
 * one status has to be able to read it.
 */
export class ApiError extends Error {
	readonly status: number;

	constructor(message: string, status: number) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
	}
}

function errorMessage(text: string, status: number): string {
	try {
		const payload = JSON.parse(text) as ApiErrorPayload;
		if (payload?.error?.message) return payload.error.message;
	} catch {}
	if (text && text.length <= 200) return text;
	return `Request failed with status ${status}`;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
	const headers: Record<string, string> = {};
	const token = getToken();
	if (token) headers['Authorization'] = `Bearer ${token}`;

	if (body && !(body instanceof FormData)) {
		headers['Content-Type'] = 'application/json';
	}

	const res = await fetch(`${BASE}${path}`, {
		method,
		headers,
		body: body ? (body instanceof FormData ? body : JSON.stringify(body)) : undefined
	});

	if (!res.ok) {
		const text = await res.text();
		if (res.status === 401 && typeof window !== 'undefined') {
			localStorage.removeItem(TOKEN_KEY);
		}
		throw new ApiError(errorMessage(text.trim(), res.status), res.status);
	}

	const contentType = res.headers.get('content-type');
	if (contentType?.includes('application/json')) {
		return res.json();
	}
	return res.text() as unknown as T;
}

export interface JardinStatus {
	machine: string;
	rules: string[];
	skills: string[];
}

export interface TokenInfo {
	token?: string;
	name: string;
	scope?: string;
	created_at: string;
	last_seen: string;
}

export interface FileEntry {
	path: string;
	checksum: string;
	size: number;
	mod_time: string;
}

export interface SessionStatRow {
	key: string;
	sessions: number;
	seconds: number;
	tokens_in: number;
	tokens_out: number;
	cache_read: number;
}

export interface SessionStats {
	by: string;
	rows: SessionStatRow[];
}

export interface SessionBlock {
	id: string;
	project: string;
	machine: string;
	agent: string;
	branch?: string;
	model?: string;
	started_at: string;
	ended_at: string;
	events: number;
	tokens_in: number;
	tokens_out: number;
	cache_read: number;
	cache_write: number;
}

export interface LiveSession {
	project: string;
	agent: string;
	branch?: string;
	model?: string;
	started_at: string;
	last_event_at: string;
	events: number;
	tokens_out: number;
	machine: string;
	live: boolean;
	machine_online: boolean;
	idle_seconds: number;
}

export interface TimelineSeries {
	key: string;
	seconds: number[];
	sessions: number[];
	tokens_in: number[];
	tokens_out: number[];
	cache_read: number[];
}

export interface SessionTimeline {
	bucket: string;
	by: string;
	labels: string[];
	series: TimelineSeries[];
}

/*
 * `resets_in_seconds` and `expired` are derived by the server on every read, never stored: a
 * recorded percentage is only true until its window rolls over, after which it is the last
 * value observed rather than the current one.
 */
export interface UsageWindow {
	key: string;
	label: string;
	used_percentage: number;
	resets_at?: string;
	resets_in_seconds?: number;
	expired?: boolean;
}

export interface UsageSnapshot {
	machine: string;
	updated_at: string;
	age_seconds?: number;
	stale?: boolean;
	source: string;
	model?: string;
	windows: UsageWindow[];
}

export interface UsageHistorySeries {
	key: string;
	label: string;
	values: (number | null)[];
}

export interface UsageHistory {
	labels: string[];
	series: UsageHistorySeries[];
}

export interface AntenneSettings {
	enabled: boolean;
	instance: string;
	secret: string;
	user_email: string;
	machine_emails: Record<string, string>;
	emit_since?: string;
	usage_alerts?: boolean;
	usage_threshold?: number;
}

export interface EmitterStatus {
	connected: boolean;
	last_error?: string;
	emitted: number;
	pending: number;
	usage_alerts_pending?: number;
}

export interface JardinSettings {
	antenne: AntenneSettings;
	status: EmitterStatus;
	/* Fields pinned by the environment; editing them would be reverted on restart. */
	env_managed?: Record<string, boolean>;
}

export interface AuthConfig {
	password_auth: boolean;
	sso_only: boolean;
	oidc_enabled: boolean;
}

export interface AuthUser {
	email: string;
	name: string;
	admin: boolean;
}

export type SpaceRole = 'owner' | 'admin' | 'member';

export interface Space {
	id: string;
	name: string;
	description: string;
	role: SpaceRole;
	created_at: string;
	updated_at: string;
}

export interface SpaceMember {
	email: string;
	name: string;
	role: SpaceRole;
	joined_at: string;
}

export interface UserInfo {
	email: string;
	name: string;
	admin: boolean;
}

export const backend = {
	status: () => request<JardinStatus>('GET', `/status${spaceQuery()}`),

	memorySearch: (query: string) =>
		request<{ path: string; line: number; content: string }[]>(
			'GET',
			`/memory/search?q=${encodeURIComponent(query)}${spaceQuery('&')}`
		),
	memoryIndex: () => request<string>('GET', `/memory/index${spaceQuery()}`),

	syncTree: () => request<FileEntry[]>('GET', '/sync/tree'),
	syncFile: (path: string) => request<string>('GET', `/sync/files/${path}`),
	syncFilePut: (path: string, content: string) => request<void>('PUT', `/sync/files/${path}`, content),
	syncFileDelete: (path: string) => request<void>('DELETE', `/sync/files/${path}`),

	rulesList: () => request<string[]>('GET', `/rules${spaceQuery()}`),
	ruleGet: (name: string) => request<string>('GET', `/rules/${name}${spaceQuery()}`),
	ruleSave: (name: string, content: string) => request<void>('PUT', `/rules/${name}${spaceQuery()}`, content),
	ruleDelete: (name: string) => request<void>('DELETE', `/rules/${name}${spaceQuery()}`),

	skillsList: () => request<string[]>('GET', `/skills${spaceQuery()}`),
	skillGet: (name: string) => request<string>('GET', `/skills/${name}${spaceQuery()}`),
	skillSave: (name: string, content: string) => request<void>('PUT', `/skills/${name}${spaceQuery()}`, content),
	skillDelete: (name: string) => request<void>('DELETE', `/skills/${name}${spaceQuery()}`),

	sessionsStats: (since: string, by: string) =>
		request<SessionStats>('GET', `/sessions/stats?since=${since}&by=${by}${spaceQuery('&')}`),
	sessionsRecent: (limit = 20) => request<SessionBlock[]>('GET', `/sessions/recent?limit=${limit}${spaceQuery('&')}`),
	sessionsLive: () => request<LiveSession[]>('GET', `/sessions/live${spaceQuery('?')}`),
	sessionsTimeline: (since: string, bucket: string, by: string) =>
		request<SessionTimeline>(
			'GET',
			`/sessions/timeline?since=${since}&bucket=${bucket}&by=${by}${spaceQuery('&')}`
		),

	usageCurrent: () => request<UsageSnapshot[]>('GET', `/usage${spaceQuery()}`),
	usageHistory: (since: string, machine?: string) =>
		request<UsageHistory>(
			'GET',
			`/usage/history?since=${since}${machine ? `&machine=${encodeURIComponent(machine)}` : ''}${spaceQuery('&')}`
		),

	settingsGet: () => request<JardinSettings>('GET', '/settings'),
	settingsSave: (antenne: AntenneSettings) => request<JardinSettings>('PUT', '/settings', { antenne }),

	tokensCreate: (name: string) => request<TokenInfo>('POST', '/tokens', { name }),
	tokensList: () => request<TokenInfo[]>('GET', '/tokens'),
	tokensDelete: (name: string) => request<void>('DELETE', `/tokens/${name}`),

	login: (password: string) => request<{ token: string }>('POST', '/auth/login', { password }),
	getAuthConfig: () => request<AuthConfig>('GET', '/auth/config'),
	authMe: () => request<AuthUser>('GET', '/auth/me'),
	logout: () => request<void>('POST', '/auth/logout'),

	spacesList: () => request<{ spaces: Space[] }>('GET', '/spaces').then((r) => r.spaces ?? []),
	spaceCreate: (name: string, description: string) => request<Space>('POST', '/spaces', { name, description }),
	spaceUpdate: (id: string, name: string, description: string) =>
		request<Space>('PUT', `/spaces/${id}`, { name, description }),
	spaceDelete: (id: string) => request<void>('DELETE', `/spaces/${id}`),
	spaceMembers: (id: string) =>
		request<{ members: SpaceMember[] }>('GET', `/spaces/${id}/members`).then((r) => r.members ?? []),
	spaceMemberAdd: (id: string, email: string, role: SpaceRole) =>
		request<unknown>('POST', `/spaces/${id}/members`, { email, role }),
	spaceMemberUpdate: (id: string, email: string, role: SpaceRole) =>
		request<unknown>('PUT', `/spaces/${id}/members/${encodeURIComponent(email)}`, { role }),
	spaceMemberRemove: (id: string, email: string) =>
		request<void>('DELETE', `/spaces/${id}/members/${encodeURIComponent(email)}`),
	spaceLeave: (id: string) => request<void>('POST', `/spaces/${id}/leave`),
	usersList: () => request<UserInfo[]>('GET', '/users'),

	deviceInfo: (code: string) =>
		request<{ user_code: string; machine: string; ip: string; status: string }>(
			'GET',
			`/auth/device/info?code=${encodeURIComponent(code)}`
		),
	deviceApprove: (userCode: string) =>
		request<{ machine: string }>('POST', '/auth/device/approve', { user_code: userCode }),
	deviceDeny: (userCode: string) =>
		request<void>('POST', '/auth/device/deny', { user_code: userCode })
};
