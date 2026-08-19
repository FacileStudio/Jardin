package docs

var (
	unauthenticated = Error{Status: 401, Code: "unauthenticated", Description: "Missing, expired or revoked token."}
	forbidden       = Error{Status: 403, Code: "permission_denied", Description: "The token is valid but not admin-scoped."}
	notFound        = Error{Status: 404, Code: "not_found", Description: "No such record."}
	badRequest      = Error{Status: 400, Code: "invalid_argument", Description: "Invalid JSON body or query."}
	internalError   = Error{Status: 500, Code: "internal", Description: "Unexpected server error."}
)

// anyToken and adminToken mirror s.auth(false, …) and s.auth(true, …): both need
// a valid machine token, but the admin scope is a strictly smaller set.
var (
	anyToken   = []Error{unauthenticated, internalError}
	adminToken = []Error{unauthenticated, forbidden, internalError}
)

var nameParam = Field{Name: "name", Type: "string", Description: "Record name, as it appears on disk."}
var claimProjectParam = Field{Name: "project", Type: "string", Description: "Project name, matched case-insensitively."}
var claimMachineParam = Field{Name: "machine", Type: "string", Description: "Machine that holds the claim."}
var claimAgentParam = Field{Name: "agent", Type: "string", Description: "Agent that holds the claim."}
var spaceParam = Field{Name: "id", Type: "string", Description: "Space ID."}
var memberParam = Field{Name: "email", Type: "string", Description: "Member's email address."}

// Registry is every route the server mounts under /api. A test walks the live
// router and fails when the two disagree, so this cannot quietly go stale.
var Registry = Response{Modules: []Module{
	{
		Name:        "auth",
		Description: "Sign-in, session identity, and the OIDC handshake with Porte.",
		Routes: []Route{
			{Method: "GET", Path: "/auth/config", Summary: "Report which sign-in methods this deployment offers", ResponseBody: "ConfigResponse", Errors: []Error{internalError}},
			{Method: "POST", Path: "/auth/login", Summary: "Sign in with a password", Description: "Rate-limited. Absent entirely when SSO_ONLY is set.", RequestBody: "LoginRequest", ResponseBody: "AuthResponse", Errors: []Error{badRequest, unauthenticated, {Status: 429, Code: "resource_exhausted", Description: "Too many failed attempts."}, internalError}},
			{Method: "GET", Path: "/auth/oidc", Summary: "Begin the OIDC handshake", Description: "Redirects to Porte. A CLI adds flow=cli, port=<1024-65535> and an optional cli_state nonce to be sent back to a loopback listener instead of the web app.", Errors: []Error{badRequest, internalError}},
			{Method: "GET", Path: "/auth/oidc/callback", Summary: "Complete the OIDC handshake", Description: "Redirects back with the session token in the URL fragment, never the query string. A CLI flow is redirected to http://127.0.0.1:<port>/ with a one-time code and the nonce it sent.", Errors: []Error{badRequest, unauthenticated, internalError}},
			{Method: "POST", Path: "/auth/oidc/exchange", Summary: "Exchange a CLI login code for a token", Description: "The code is single-use and valid for sixty seconds.", RequestBody: "ExchangeRequest", ResponseBody: "AuthResponse", Errors: []Error{badRequest, unauthenticated, internalError}},
			{Method: "GET", Path: "/auth/me", Summary: "Return the caller's identity", Auth: "bearer", ResponseBody: "MeResponse", Errors: anyToken},
			{Method: "POST", Path: "/auth/logout", Summary: "Revoke the caller's session", Auth: "bearer", Errors: anyToken},
		},
	},
	{
		Name:        "device",
		Description: "Device-code flow: how a CLI on a new machine gets a token without a browser redirect.",
		Routes: []Route{
			{Method: "POST", Path: "/auth/device/start", Summary: "Start a device authorization", ResponseBody: "DeviceStartResponse", Errors: []Error{badRequest, internalError}},
			{Method: "POST", Path: "/auth/device/poll", Summary: "Poll for approval", Description: "Returns the token once a signed-in admin approves the request.", RequestBody: "DevicePollRequest", ResponseBody: "AuthResponse", Errors: []Error{badRequest, {Status: 428, Code: "failed_precondition", Description: "Still pending approval."}, internalError}},
			{Method: "GET", Path: "/auth/device/info", Summary: "Describe a pending device request", Auth: "bearer (admin)", ResponseBody: "DeviceInfoResponse", Errors: append(adminToken, notFound)},
			{Method: "POST", Path: "/auth/device/approve", Summary: "Approve a device request", Auth: "bearer (admin)", Errors: append(adminToken, notFound)},
			{Method: "POST", Path: "/auth/device/deny", Summary: "Deny a device request", Auth: "bearer (admin)", Errors: append(adminToken, notFound)},
		},
	},
	{
		Name:        "status",
		Description: "What this Mycelium instance holds.",
		Routes: []Route{
			{Method: "GET", Path: "/status", Summary: "Summarize the instance", Description: "Counts of memory pages, rules and skills, plus the sync generation.", Auth: "bearer", ResponseBody: "StatusResponse", Errors: anyToken},
		},
	},
	{
		Name:        "memory",
		Description: "Search and routing over the wiki, without transferring it.",
		Routes: []Route{
			{Method: "GET", Path: "/memory/search", Summary: "Search memory pages", Description: "Full-text search over the wiki; `q` is the query.", Auth: "bearer", ResponseBody: "[]SearchResult", Errors: append(anyToken, badRequest)},
			{Method: "POST", Path: "/memory/search", Summary: "Search memory pages, lexically and semantically", Description: "Fuses the BM25 ranking with a vector ranking using Reciprocal Rank Fusion, and returns ranked chunks rather than lines. `space_id` scopes the search to a space the caller belongs to. `degraded` is true when the embedding backend or its index was absent or unreachable and the answer is lexical-only — never an error.", Auth: "bearer", RequestBody: "MemorySearchRequest", ResponseBody: "MemorySearchResponse", Errors: append(anyToken, badRequest, forbidden)},
			{Method: "GET", Path: "/memory/index", Summary: "Return the index", Description: "The one-line-per-page router that decides what to read next.", Auth: "bearer", ResponseBody: "IndexResponse", Errors: anyToken},
			{Method: "GET", Path: "/memory/index/status", Summary: "Report embedding index progress", Description: "Progress of the vector index behind semantic search, for a progress bar: `enabled` is false with every other field zeroed when no embedding backend is configured. `total_chunks` is what the wiki holds, `indexed_chunks` what the store holds, and `chunks_per_second`/`eta_seconds` are measured over the pass in flight and are zero when idle. `last_error` carries the most recent embedding failure, so a model outage is visible rather than a stalled bar. `space_id` scopes the counts to a space the caller belongs to.", Auth: "bearer", ResponseBody: "MemoryIndexStatusResponse", Errors: append(anyToken, forbidden)},
		},
	},
	{
		Name:        "sessions",
		Description: "Agent session telemetry across every machine.",
		Routes: []Route{
			{Method: "GET", Path: "/sessions/stats", Summary: "Aggregate session statistics", Auth: "bearer", ResponseBody: "StatsResponse", Errors: anyToken},
			{Method: "GET", Path: "/sessions/recent", Summary: "List recent sessions", Auth: "bearer", ResponseBody: "[]Session", Errors: anyToken},
			{Method: "GET", Path: "/sessions/live", Summary: "List sessions currently active", Auth: "bearer", ResponseBody: "[]Session", Errors: anyToken},
			{Method: "GET", Path: "/sessions/timeline", Summary: "Bucket session activity over time", Description: "Gap-filled buckets for charts. `since` (7d, 30d, 12h, all — default 30d), `bucket` (day or month), `by` (project, machine, agent, branch, model, total). Series are capped, with the remainder folded into `Other`.", Auth: "bearer", ResponseBody: "TimelineResponse", Errors: append(anyToken, badRequest)},
		},
	},
	{
		Name:        "claims",
		Description: "In-flight task leases, so a second agent can see or take over work already claimed on a repo.",
		Routes: []Route{
			{Method: "GET", Path: "/claims", Summary: "List every active claim", Description: "Liveness is resolved against the clock at read time, not stored.", Auth: "bearer", ResponseBody: "[]ClaimEntry", Errors: anyToken},
			{Method: "DELETE", Path: "/claims/{project}/{machine}/{agent}", Summary: "Release a claim", Description: "Releasing an already-absent claim is not an error.", Auth: "bearer", PathParams: []Field{claimProjectParam, claimMachineParam, claimAgentParam}, Errors: anyToken},
		},
	},
	{
		Name:        "usage",
		Description: "Subscription rate-limit windows, as reported by Claude Code on each machine.",
		Routes: []Route{
			{Method: "GET", Path: "/usage", Summary: "Read each machine's latest usage snapshot", Description: "Empty array when nothing has been recorded yet.", Auth: "bearer", ResponseBody: "[]UsageSnapshot", Errors: anyToken},
			{Method: "GET", Path: "/usage/history", Summary: "Read recorded usage samples over time", Description: "`since` defaults to 7d; `machine` filters to one machine. Samples are irregular, so labels are the sample instants and a missing window carries null.", Auth: "bearer", ResponseBody: "UsageHistory", Errors: append(anyToken, badRequest)},
		},
	},
	{
		Name:        "settings",
		Description: "Instance-wide configuration.",
		Routes: []Route{
			{Method: "GET", Path: "/settings", Summary: "Read the settings", Auth: "bearer (admin)", ResponseBody: "Settings", Errors: adminToken},
			{Method: "PUT", Path: "/settings", Summary: "Replace the settings", Auth: "bearer (admin)", RequestBody: "Settings", ResponseBody: "Settings", Errors: append(adminToken, badRequest)},
		},
	},
	{
		Name:        "rules",
		Description: "The agent rules Mycelium renders into each tool's native config.",
		Routes: []Route{
			{Method: "GET", Path: "/rules", Summary: "List rules", Auth: "bearer", ResponseBody: "[]Rule", Errors: anyToken},
			{Method: "GET", Path: "/rules/{name}", Summary: "Read one rule", Auth: "bearer", PathParams: []Field{nameParam}, ResponseBody: "Rule", Errors: append(anyToken, notFound)},
			{Method: "PUT", Path: "/rules/{name}", Summary: "Create or replace a rule", Auth: "bearer", PathParams: []Field{nameParam}, RequestBody: "Rule", ResponseBody: "Rule", Errors: append(anyToken, badRequest)},
			{Method: "DELETE", Path: "/rules/{name}", Summary: "Delete a rule", Auth: "bearer", PathParams: []Field{nameParam}, Errors: append(anyToken, notFound)},
		},
	},
	{
		Name:        "skills",
		Description: "The skills Mycelium installs into each agent.",
		Routes: []Route{
			{Method: "GET", Path: "/skills", Summary: "List skills", Auth: "bearer", ResponseBody: "[]Skill", Errors: anyToken},
			{Method: "GET", Path: "/skills/{name}", Summary: "Read one skill", Auth: "bearer", PathParams: []Field{nameParam}, ResponseBody: "Skill", Errors: append(anyToken, notFound)},
			{Method: "PUT", Path: "/skills/{name}", Summary: "Create or replace a skill", Auth: "bearer", PathParams: []Field{nameParam}, RequestBody: "Skill", ResponseBody: "Skill", Errors: append(anyToken, badRequest)},
			{Method: "DELETE", Path: "/skills/{name}", Summary: "Delete a skill", Auth: "bearer", PathParams: []Field{nameParam}, Errors: append(anyToken, notFound)},
		},
	},
	{
		Name:        "flows",
		Description: "Recorded shell procedures. Trust pins and run history are per-machine and never reach the server; this is the definition only.",
		Routes: []Route{
			{Method: "GET", Path: "/flows", Summary: "List flows", Auth: "bearer", ResponseBody: "[]string", Errors: anyToken},
			{Method: "GET", Path: "/flows/{name}", Summary: "Read one flow", Auth: "bearer", PathParams: []Field{nameParam}, ResponseBody: "FlowDetail", Errors: append(anyToken, notFound)},
		},
	},
	{
		Name:        "models",
		Description: "Typed step extensions under extensions/models. The server has no bun and no shell, so a model is served as raw source, never executed. Reading one is a wildcard route (GET /models/{path}, where path may contain slashes) and is not listed here for the same reason /sync/files/* is not.",
		Routes: []Route{
			{Method: "GET", Path: "/models", Summary: "List models", Auth: "bearer", ResponseBody: "[]ModelInfo", Errors: anyToken},
		},
	},
	{
		Name:        "users",
		Description: "Who can sign in to this instance.",
		Routes: []Route{
			{Method: "GET", Path: "/users", Summary: "List users", Auth: "bearer", ResponseBody: "[]User", Errors: anyToken},
		},
	},
	{
		Name:        "spaces",
		Description: "Shared memory spaces and their membership.",
		Routes: []Route{
			{Method: "GET", Path: "/spaces", Summary: "List spaces the caller belongs to", Auth: "bearer", ResponseBody: "[]Space", Errors: anyToken},
			{Method: "POST", Path: "/spaces", Summary: "Create a space", Auth: "bearer", RequestBody: "CreateSpaceRequest", ResponseBody: "Space", Errors: append(anyToken, badRequest)},
			{Method: "PUT", Path: "/spaces/{id}", Summary: "Update a space", Auth: "bearer", PathParams: []Field{spaceParam}, RequestBody: "UpdateSpaceRequest", ResponseBody: "Space", Errors: append(anyToken, badRequest, notFound)},
			{Method: "DELETE", Path: "/spaces/{id}", Summary: "Delete a space", Auth: "bearer", PathParams: []Field{spaceParam}, Errors: append(anyToken, forbidden, notFound)},
			{Method: "GET", Path: "/spaces/{id}/members", Summary: "List a space's members", Auth: "bearer", PathParams: []Field{spaceParam}, ResponseBody: "[]SpaceMember", Errors: append(anyToken, notFound)},
			{Method: "POST", Path: "/spaces/{id}/members", Summary: "Add a member", Auth: "bearer", PathParams: []Field{spaceParam}, RequestBody: "AddMemberRequest", ResponseBody: "SpaceMember", Errors: append(anyToken, badRequest, forbidden, notFound)},
			{Method: "PUT", Path: "/spaces/{id}/members/{email}", Summary: "Change a member's role", Auth: "bearer", PathParams: []Field{spaceParam, memberParam}, RequestBody: "UpdateMemberRequest", ResponseBody: "SpaceMember", Errors: append(anyToken, badRequest, forbidden, notFound)},
			{Method: "DELETE", Path: "/spaces/{id}/members/{email}", Summary: "Remove a member", Auth: "bearer", PathParams: []Field{spaceParam, memberParam}, Errors: append(anyToken, forbidden, notFound)},
			{Method: "POST", Path: "/spaces/{id}/leave", Summary: "Leave a space", Auth: "bearer", PathParams: []Field{spaceParam}, Errors: append(anyToken, notFound)},
		},
	},
	{
		Name:        "tokens",
		Description: "Per-machine sync tokens. Stored sha256-hashed, so a created token is shown once and never again.",
		Routes: []Route{
			{Method: "GET", Path: "/tokens", Summary: "List tokens", Description: "Metadata only — the token values are not recoverable.", Auth: "bearer (admin)", ResponseBody: "[]Token", Errors: adminToken},
			{Method: "POST", Path: "/tokens", Summary: "Issue a token", Description: "Returns the value once. Scope is admin or sync.", Auth: "bearer (admin)", RequestBody: "CreateTokenRequest", ResponseBody: "CreatedToken", Errors: append(adminToken, badRequest)},
			{Method: "DELETE", Path: "/tokens/{name}", Summary: "Revoke a token", Auth: "bearer (admin)", PathParams: []Field{nameParam}, Errors: append(adminToken, notFound)},
		},
	},
	{
		Name:        "sync",
		Description: "The file transfer the CLI runs. Paths are wiki-relative and may contain slashes, so these routes take a trailing wildcard rather than a named parameter.",
		Routes: []Route{
			{Method: "GET", Path: "/sync/tree", Summary: "List every file and its hash", Description: "The client diffs this against its own tree to decide what to move.", Auth: "bearer", ResponseBody: "TreeResponse", Errors: anyToken},
		},
	},
}}
