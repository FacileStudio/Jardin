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
		throw new Error(text || res.statusText);
	}

	const contentType = res.headers.get('content-type');
	if (contentType?.includes('application/json')) {
		return res.json();
	}
	return res.text() as unknown as T;
}

export interface CellInfo {
	name: string;
	path: string;
}

export interface RucheStatus {
	active_cell: string;
	machine: string;
	url: string;
	cells: CellInfo[];
	rules: string[];
	skills: string[];
}

export interface FileEntry {
	path: string;
	checksum: string;
	size: number;
	mod_time: string;
}

export interface TokenInfo {
	token: string;
	name: string;
	created_at: string;
}

export const backend = {
	status: () => request<RucheStatus>('GET', '/status'),

	cells: () => request<string[]>('GET', '/cells'),
	createCell: (name: string) => request<void>('POST', '/cells', { name }),
	useCell: (name: string) => request<void>('POST', '/cells/use', { name }),

	tree: (cell: string) => request<FileEntry[]>('GET', `/cells/${cell}/tree`),
	getFile: (cell: string, path: string) => request<string>('GET', `/cells/${cell}/files/${path}`),
	putFile: (cell: string, path: string, content: string) =>
		request<void>('PUT', `/cells/${cell}/files/${path}`, content),
	deleteFile: (cell: string, path: string) => request<void>('DELETE', `/cells/${cell}/files/${path}`),

	brainSearch: (query: string) =>
		request<{ path: string; line: number; content: string }[]>('GET', `/brain/search?q=${encodeURIComponent(query)}`),
	brainIndex: () => request<string>('GET', '/brain/index'),

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
	getAuthConfig: () => request<{ sso_only: boolean; oidc_enabled: boolean }>('GET', '/auth/config')
};
