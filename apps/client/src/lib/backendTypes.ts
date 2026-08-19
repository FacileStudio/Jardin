export interface MyceliumStatus {
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

/*
 * `chunks_per_second` and `eta_seconds` are the server's own measurement of the run, never
 * timed by the client from two samples. `enabled: false` is the normal state of a wiki with
 * no embedding model configured, and zeroes the rest of the payload rather than omitting it.
 */
export interface MemorySearchHit {
	path: string;
	heading: string;
	line: number;
	score: number;
	excerpt: string;
}

export interface MemorySearchResponse {
	results: MemorySearchHit[];
	degraded: boolean;
}

export interface MemoryIndexModel {
	name: string;
	digest: string;
	dims: number;
}

export interface MemoryIndexState {
	enabled: boolean;
	model?: MemoryIndexModel;
	store: string;
	total_chunks: number;
	indexed_chunks: number;
	pending_paths: number;
	indexing: boolean;
	started_at?: string;
	updated_at?: string;
	chunks_per_second: number;
	eta_seconds: number;
	last_error: string;
}

export interface SessionStatRow {
	key: string;
	sessions: number;
	seconds: number;
	tokens_in: number;
	tokens_out: number;
	cache_read: number;
	cost_total: number;
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
	cost_input: number;
	cost_output: number;
	cost_total: number;
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

export interface Claim {
	project: string;
	machine: string;
	agent: string;
	branch?: string;
	task: string;
	started_at: string;
	updated_at: string;
	body?: string;
	live: boolean;
	machine_online: boolean;
}

export interface TimelineSeries {
	key: string;
	seconds: number[];
	sessions: number[];
	tokens_in: number[];
	tokens_out: number[];
	cache_read: number[];
	cost_total: number[];
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

export interface MyceliumSettings {
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

export interface FlowStepSummary {
	name: string;
	kind: 'run' | 'type';
	type?: string;
	depends_on?: string[];
	needs?: Record<string, string>;
}

export interface FlowSummary {
	name: string;
	description?: string;
	steps: FlowStepSummary[];
}

/*
 * Trust pins and run history live under a dotfile and runs/, neither of which syncs
 * (internal/sync's syncSkip excludes both) — a flow that fails to parse still has to render,
 * since an author still needs to see and fix it, so parse_error stands in for summary rather
 * than the request failing.
 */
export interface FlowDetail {
	raw: string;
	summary?: FlowSummary;
	parse_error?: string;
}

export interface ModelInfo {
	type: string;
	path: string;
}
