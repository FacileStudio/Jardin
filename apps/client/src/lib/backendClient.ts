import { getActiveSpaceId } from './space.svelte';

const BASE = '/api';

export const TOKEN_KEY = 'jardin.token';

export function spaceQuery(sep: '?' | '&' = '?'): string {
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

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
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
