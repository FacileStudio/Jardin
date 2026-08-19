import { getActiveSpaceId } from './space.svelte';
import { request, spaceQuery } from './backendClient';
import type {
	AntenneSettings,
	AuthConfig,
	AuthUser,
	Claim,
	FileEntry,
	FlowDetail,
	JardinSettings,
	JardinStatus,
	LiveSession,
	MemoryIndexState,
	MemorySearchResponse,
	ModelInfo,
	SessionBlock,
	SessionStats,
	SessionTimeline,
	Space,
	SpaceMember,
	SpaceRole,
	TokenInfo,
	UsageHistory,
	UsageSnapshot,
	UserInfo
} from './backendTypes';

export * from './backendTypes';
export { ApiError, TOKEN_KEY } from './backendClient';

export const backend = {
	status: () => request<JardinStatus>('GET', `/status${spaceQuery()}`),

	memorySearch: (query: string, limit = 20) =>
		request<MemorySearchResponse>('POST', `/memory/search`, {
			query,
			limit,
			space_id: getActiveSpaceId() ?? ''
		}),
	memoryIndex: () => request<string>('GET', `/memory/index${spaceQuery()}`),
	memoryIndexStatus: () => request<MemoryIndexState>('GET', `/memory/index/status${spaceQuery()}`),

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

	flowsList: () => request<string[]>('GET', `/flows${spaceQuery()}`),
	flowGet: (name: string) => request<FlowDetail>('GET', `/flows/${name}${spaceQuery()}`),

	modelsList: () => request<ModelInfo[]>('GET', `/models${spaceQuery()}`),
	modelGet: (path: string) => request<string>('GET', `/models/${path}${spaceQuery()}`),

	sessionsStats: (since: string, by: string) =>
		request<SessionStats>('GET', `/sessions/stats?since=${since}&by=${by}${spaceQuery('&')}`),
	sessionsRecent: (limit = 20) => request<SessionBlock[]>('GET', `/sessions/recent?limit=${limit}${spaceQuery('&')}`),
	sessionsLive: () => request<LiveSession[]>('GET', `/sessions/live${spaceQuery('?')}`),
	claimsList: () => request<Claim[]>('GET', `/claims${spaceQuery('?')}`),
	claimRelease: (project: string, machine: string, agent: string) =>
		request<void>(
			'DELETE',
			`/claims/${encodeURIComponent(project)}/${encodeURIComponent(machine)}/${encodeURIComponent(agent)}${spaceQuery('?')}`
		),
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
