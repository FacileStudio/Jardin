const BASE = '/api';

function getToken(): string | null {
	if (typeof window === 'undefined') return null;
	return localStorage.getItem('ruche.token');
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
			localStorage.removeItem('ruche.token');
		}
		throw new Error(text || res.statusText);
	}

	const contentType = res.headers.get('content-type');
	if (contentType?.includes('application/json')) {
		return res.json();
	}
	return res.text() as unknown as T;
}

export interface RucheStatus {
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

export const backend = {
	status: () => request<RucheStatus>('GET', '/status'),

	memorySearch: (query: string) =>
		request<{ path: string; line: number; content: string }[]>('GET', `/memory/search?q=${encodeURIComponent(query)}`),
	memoryIndex: () => request<string>('GET', '/memory/index'),

	syncTree: () => request<FileEntry[]>('GET', '/sync/tree'),
	syncFile: (path: string) => request<string>('GET', `/sync/files/${path}`),

	rulesList: () => request<string[]>('GET', '/rules'),
	ruleGet: (name: string) => request<string>('GET', `/rules/${name}`),
	ruleSave: (name: string, content: string) => request<void>('PUT', `/rules/${name}`, content),
	ruleDelete: (name: string) => request<void>('DELETE', `/rules/${name}`),

	skillsList: () => request<string[]>('GET', '/skills'),
	skillGet: (name: string) => request<string>('GET', `/skills/${name}`),
	skillSave: (name: string, content: string) => request<void>('PUT', `/skills/${name}`, content),
	skillDelete: (name: string) => request<void>('DELETE', `/skills/${name}`),

	tokensCreate: (name: string) => request<TokenInfo>('POST', '/tokens', { name }),
	tokensList: () => request<TokenInfo[]>('GET', '/tokens'),
	tokensDelete: (name: string) => request<void>('DELETE', `/tokens/${name}`),

	login: (password: string) => request<{ token: string }>('POST', '/auth/login', { password }),
	getAuthConfig: () => request<{ sso_only: boolean; oidc_enabled: boolean }>('GET', '/auth/config'),

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
