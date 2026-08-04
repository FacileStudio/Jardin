import { getActiveSpaceId } from './space.svelte';

const BASE = '/api';

function spaceQuery(sep: '?' | '&' = '?'): string {
	const id = getActiveSpaceId();
	return id ? `${sep}space_id=${encodeURIComponent(id)}` : '';
}

function getToken(): string | null {
	if (typeof window === 'undefined') return null;
	return localStorage.getItem('jardin.token');
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
			localStorage.removeItem('jardin.token');
		}
		throw new Error(text || res.statusText);
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

export interface NookSettings {
	enabled: boolean;
	instance: string;
	secret: string;
	user_email: string;
	machine_emails: Record<string, string>;
	emit_since?: string;
}

export interface EmitterStatus {
	connected: boolean;
	last_error?: string;
	emitted: number;
	pending: number;
}

export interface JardinSettings {
	nook: NookSettings;
	status: EmitterStatus;
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

	settingsGet: () => request<JardinSettings>('GET', '/settings'),
	settingsSave: (nook: NookSettings) => request<JardinSettings>('PUT', '/settings', { nook }),

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
