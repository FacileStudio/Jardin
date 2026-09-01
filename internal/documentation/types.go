// Package docs holds Mycelium's hand-written route registry, the single source
// the reference page at /docs and its OpenAPI document are both built from.
//
// The types are aliases of tronc/apiref's, so the registry is exactly the
// suite-wide shape and apiref.Undocumented can check it against the live router.
//
// The package is named docs rather than documentation because go/build ignores
// every file declaring `package documentation` — a godoc convention for
// documentation-only directories — which makes the package silently vanish with
// "build constraints exclude all Go files".
package docs

import "github.com/FacileStudio/tronc/apiref"

// The registry types mirror tronc/apiref's so the registry is the suite-wide
// shape apiref.Undocumented can check against the live router.
type (
	Response = apiref.Registry
	Module   = apiref.Module
	Route    = apiref.Route
	Field    = apiref.Field
	Error    = apiref.Error
)

type ConfigJournal struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

type ConfigResponse struct {
	PasswordAuth  bool           `json:"password_auth"`
	SSOOnly       bool           `json:"sso_only"`
	OIDCEnabled   bool           `json:"oidc_enabled"`
	DeviceEnabled bool           `json:"device_enabled"`
	Journal       *ConfigJournal `json:"journal,omitempty"`
}

type LoginRequest struct {
	Password string `json:"password"`
	Machine  string `json:"machine,omitempty"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

type ExchangeRequest struct {
	Code string `json:"code"`
}

type MeResponse struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Admin bool   `json:"admin"`
}

type DeviceStartRequest struct {
	Machine string `json:"machine,omitempty"`
}

type DeviceStartResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	Machine                 string `json:"machine"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expires_in"`
}

type DevicePollRequest struct {
	DeviceCode string `json:"device_code"`
}

type DeviceInfoResponse struct {
	UserCode string `json:"user_code"`
	Machine  string `json:"machine"`
	IP       string `json:"ip"`
	Status   string `json:"status"`
}

type DeviceApproveRequest struct {
	UserCode string `json:"user_code"`
}

type DeviceApproveResponse struct {
	Machine string `json:"machine"`
}

type DeviceDenyRequest struct {
	UserCode string `json:"user_code"`
}

type StatusResponse struct {
	Machine string   `json:"machine,omitempty"`
	Rules   []string `json:"rules"`
	Skills  []string `json:"skills"`
}

type SearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

type MemorySearchRequest struct {
	Query   string `json:"query"`
	SpaceID string `json:"space_id,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type MemoryHit struct {
	Path    string  `json:"path"`
	Heading string  `json:"heading"`
	Line    int     `json:"line"`
	Score   float64 `json:"score"`
	Excerpt string  `json:"excerpt"`
	Date    string  `json:"date,omitempty"`
}

type MemorySearchResponse struct {
	Results  []MemoryHit `json:"results"`
	Degraded bool        `json:"degraded"`
}

type IndexResponse struct {
	Content string `json:"content"`
}

type MemoryIndexStatusResponse struct {
	Enabled         bool    `json:"enabled"`
	Model           string  `json:"model,omitempty"`
	Store           string  `json:"store"`
	TotalChunks     int     `json:"total_chunks"`
	IndexedChunks   int     `json:"indexed_chunks"`
	PendingPaths    int     `json:"pending_paths"`
	Indexing        bool    `json:"indexing"`
	StartedAt       string  `json:"started_at"`
	UpdatedAt       string  `json:"updated_at"`
	ChunksPerSecond float64 `json:"chunks_per_second"`
	ETASeconds      int     `json:"eta_seconds"`
	LastError       string  `json:"last_error,omitempty"`
}

type StatRow struct {
	Key       string  `json:"key"`
	Sessions  int     `json:"sessions"`
	Seconds   int64   `json:"seconds"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	CacheRead int64   `json:"cache_read"`
	CostTotal float64 `json:"cost_total"`
}

type StatsResponse struct {
	By   string    `json:"by"`
	Rows []StatRow `json:"rows"`
}

type Session struct {
	ID         string  `json:"id"`
	Project    string  `json:"project"`
	Machine    string  `json:"machine"`
	Agent      string  `json:"agent"`
	Branch     string  `json:"branch,omitempty"`
	Model      string  `json:"model,omitempty"`
	StartedAt  string  `json:"started_at"`
	EndedAt    string  `json:"ended_at"`
	Events     int     `json:"events"`
	TokensIn   int64   `json:"tokens_in"`
	TokensOut  int64   `json:"tokens_out"`
	CacheRead  int64   `json:"cache_read"`
	CacheWrite int64   `json:"cache_write"`
	CostInput  float64 `json:"cost_input"`
	CostOutput float64 `json:"cost_output"`
	CostTotal  float64 `json:"cost_total"`
}

type LiveBlock struct {
	Project     string `json:"project"`
	Agent       string `json:"agent"`
	Branch      string `json:"branch,omitempty"`
	Model       string `json:"model,omitempty"`
	StartedAt   string `json:"started_at"`
	LastEventAt string `json:"last_event_at"`
	Events      int    `json:"events"`
	TokensOut   int64  `json:"tokens_out"`
}

type LiveEntry struct {
	LiveBlock
	Machine       string `json:"machine"`
	Live          bool   `json:"live"`
	MachineOnline bool   `json:"machine_online"`
	IdleSeconds   int64  `json:"idle_seconds"`
}

type TimelineSeries struct {
	Key       string    `json:"key"`
	Seconds   []int64   `json:"seconds"`
	Sessions  []int     `json:"sessions"`
	TokensIn  []int64   `json:"tokens_in"`
	TokensOut []int64   `json:"tokens_out"`
	CacheRead []int64   `json:"cache_read"`
	CostTotal []float64 `json:"cost_total"`
}

type TimelineResponse struct {
	Bucket string           `json:"bucket"`
	By     string           `json:"by"`
	Labels []string         `json:"labels"`
	Series []TimelineSeries `json:"series"`
}

type Claim struct {
	Project   string `json:"project"`
	Machine   string `json:"machine"`
	Agent     string `json:"agent"`
	Branch    string `json:"branch,omitempty"`
	Task      string `json:"task"`
	StartedAt string `json:"started_at"`
	UpdatedAt string `json:"updated_at"`
	Body      string `json:"body,omitempty"`
}

type ClaimEntry struct {
	Claim
	Live          bool `json:"live"`
	MachineOnline bool `json:"machine_online"`
}

type WindowView struct {
	Key             string  `json:"key"`
	Label           string  `json:"label"`
	UsedPercentage  float64 `json:"used_percentage"`
	ResetsAt        *string `json:"resets_at,omitempty"`
	ResetsInSeconds *int64  `json:"resets_in_seconds,omitempty"`
	Expired         bool    `json:"expired"`
}

type UsageSnapshot struct {
	Machine    string       `json:"machine"`
	UpdatedAt  string       `json:"updated_at"`
	AgeSeconds int64        `json:"age_seconds"`
	Stale      bool         `json:"stale"`
	Source     string       `json:"source"`
	Model      string       `json:"model,omitempty"`
	Windows    []WindowView `json:"windows"`
}

type HistorySeries struct {
	Key    string     `json:"key"`
	Label  string     `json:"label"`
	Values []*float64 `json:"values"`
}

type UsageHistory struct {
	Labels []string        `json:"labels"`
	Series []HistorySeries `json:"series"`
}

type AntenneSettings struct {
	Enabled        bool              `json:"enabled"`
	Instance       string            `json:"instance"`
	Secret         string            `json:"secret"`
	UserEmail      string            `json:"user_email"`
	MachineEmails  map[string]string `json:"machine_emails,omitempty"`
	EmitSince      string            `json:"emit_since,omitempty"`
	UsageAlerts    bool              `json:"usage_alerts"`
	UsageThreshold float64           `json:"usage_threshold,omitempty"`
}

type EmitterStatus struct {
	Connected          bool   `json:"connected"`
	LastError          string `json:"last_error,omitempty"`
	Emitted            int    `json:"emitted"`
	Pending            int    `json:"pending"`
	UsageAlertsPending int    `json:"usage_alerts_pending"`
}

type SettingsResponse struct {
	Antenne    AntenneSettings `json:"antenne"`
	Status     EmitterStatus   `json:"status"`
	EnvManaged map[string]bool `json:"env_managed"`
}

type Settings struct {
	Antenne AntenneSettings  `json:"antenne"`
	Legacy  *AntenneSettings `json:"nook,omitempty"`
}

type Rule struct {
	Content string `json:"content"`
}

type Skill struct {
	Content string `json:"content"`
}

type FlowStepSummary struct {
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
	Type      string            `json:"type,omitempty"`
	DependsOn []string          `json:"depends_on,omitempty"`
	Needs     map[string]string `json:"needs,omitempty"`
}

type FlowSummary struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Steps       []FlowStepSummary `json:"steps"`
}

type FlowDetail struct {
	Raw        string       `json:"raw"`
	Summary    *FlowSummary `json:"summary,omitempty"`
	ParseError string       `json:"parse_error,omitempty"`
}

type ModelInfo struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type ArtifactSummary struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Machine string `json:"machine"`
	Format  string `json:"format"`
	Created string `json:"created"`
	Expires string `json:"expires,omitempty"`
	Expired bool   `json:"expired"`
}

type ArtifactDetail struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Machine string `json:"machine"`
	Format  string `json:"format"`
	Created string `json:"created"`
	Expires string `json:"expires,omitempty"`
	Expired bool   `json:"expired"`
	Content string `json:"content"`
}

type User struct {
	Email     string `json:"email"`
	Name      string `json:"name"`
	Admin     bool   `json:"admin"`
	CreatedAt string `json:"created_at"`
}

type UpdateUserRequest struct {
	Name  *string `json:"name,omitempty"`
	Admin *bool   `json:"admin,omitempty"`
}

type SpaceResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type SpacesListResponse struct {
	Spaces []SpaceResponse `json:"spaces"`
}

type CreateSpaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type UpdateSpaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type MemberResponse struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	JoinedAt string `json:"joined_at"`
}

type SpaceMembersResponse struct {
	Members []MemberResponse `json:"members"`
}

type AddMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
}

type UpdateMemberRequest struct {
	Role string `json:"role"`
}

type TokenInfo struct {
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	CreatedAt string `json:"created_at"`
	LastSeen  string `json:"last_seen"`
}

type CreateTokenRequest struct {
	Name      string `json:"name"`
	UserEmail string `json:"user_email,omitempty"`
}

type CreatedToken struct {
	Token     string `json:"token"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	CreatedAt string `json:"created_at"`
}

type FileEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
}

// Reference is the configuration the /docs page and its OpenAPI document are
// served from. Both internal/server and the drift test read it, so the document
// under test is the document that ships.
func Reference() apiref.Config {
	return apiref.Config{
		Title:       "Mycelium API",
		Description: "Shared agent memory: the wiki, rules and skills that every machine and agent syncs against.",
		Servers:     []string{"/api"},
		Registry:    Registry,
	}
}
