package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"forge-drop/internal/ids"
)

type Store struct {
	sql *sql.DB
}

type UnreferencedArtifact struct {
	ID         string
	StoredPath string
	SizeBytes  int64
}

func (s *Store) ListUnreferencedArtifacts(ctx context.Context, limit int) ([]UnreferencedArtifact, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.sql.QueryContext(ctx, `SELECT a.id, a.stored_path, a.size_bytes
		FROM artifacts a
		LEFT JOIN snapshot_slots ss ON ss.artifact_id=a.id
		WHERE ss.artifact_id IS NULL
		ORDER BY a.created_at ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnreferencedArtifact
	for rows.Next() {
		var a UnreferencedArtifact
		if err := rows.Scan(&a.ID, &a.StoredPath, &a.SizeBytes); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteArtifactByID(ctx context.Context, artifactID string) error {
	_, err := s.sql.ExecContext(ctx, `DELETE FROM artifacts WHERE id=?`, artifactID)
	return err
}

func (s *Store) ListOrphanSnapshots(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.sql.QueryContext(ctx, `SELECT sn.id
		FROM snapshots sn
		LEFT JOIN snapshot_slots ss ON ss.snapshot_id=sn.id
		LEFT JOIN envs e ON e.current_snapshot_id=sn.id AND e.deleted_at IS NULL
		WHERE ss.snapshot_id IS NULL AND e.id IS NULL
		ORDER BY sn.created_at ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSnapshotByID(ctx context.Context, snapshotID string) error {
	_, err := s.sql.ExecContext(ctx, `DELETE FROM snapshots WHERE id=?`, snapshotID)
	return err
}

func NewStore(sqlDB *sql.DB) *Store {
	return &Store{sql: sqlDB}
}

type User struct {
	ID           string
	Username     string
	PasswordHash []byte
	CreatedAt    time.Time
}

type Session struct {
	ID         string
	TokenHash  []byte
	UserID     string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
}

type APIToken struct {
	ID        string
	Name      string
	Prefix    string
	TokenHash []byte
	CreatedAt time.Time
	RevokedAt *time.Time
}

type Repo struct {
	ID            string
	FullName      string
	Slug          string
	WebhookSecret string
	CreatedAt     time.Time
}

type App struct {
	ID             string
	AppKey         string
	Name           string
	DeployStrategy string
	CreatedAt      time.Time
}

type Service struct {
	ID               string
	AppID            string
	ServiceKey       string
	Name             string
	Image            string
	Command          string
	ContainerPort    int
	RunUser          string
	Env              map[string]string
	ProdHost         string
	TraefikEntrypnts string
	ComposeTemplate  string // Docker Compose YAML template
	UseCompose       bool   // If true, use compose_template instead of manual Docker API
	Revision         int
	Enabled          bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Slot struct {
	ID            string
	ServiceID     string
	SlotKey       string
	Name          string
	RepoID        string
	ContainerPath string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Env struct {
	ID              string
	AppID           string
	Kind            string // named|preview
	Name            string // prod/staging/preview
	RepoID          *string
	PRNumber        *int
	CurrentSnapshot *string
	CreatedAt       time.Time
	DeletedAt       *time.Time
	RepoFullName    *string // expanded (join), optional
	RepoSlug        *string // expanded (join), optional
}

type Artifact struct {
	ID               string
	AppID            string
	ServiceID        string
	SlotID           string
	RepoID           string
	SHA              string
	Ref              string
	PRNumber         *int
	OriginalFilename string
	SizeBytes        int64
	SHA256Hex        string
	StoredPath       string
	CreatedAt        time.Time
}

type Snapshot struct {
	ID              string
	EnvID           string
	CreatedAt       time.Time
	CreatedByUserID *string
	CreatedByToken  *string
	Note            string
}

func TokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func TokenPrefix(token string) string {
	if len(token) < 8 {
		return token
	}
	return token[:8]
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var c int
	if err := s.sql.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&c); err != nil {
		return 0, err
	}
	return c, nil
}

func (s *Store) CreateUser(ctx context.Context, username string, passwordHash []byte) (*User, error) {
	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	if _, err := s.sql.ExecContext(ctx, `INSERT INTO users(id, username, password_hash, created_at) VALUES(?,?,?, datetime('now'))`, id, username, passwordHash); err != nil {
		return nil, err
	}
	return s.GetUserByID(ctx, id)
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*User, error) {
	var u User
	var createdAt string
	if err := s.sql.QueryRowContext(ctx, `SELECT id, username, password_hash, created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ct, err := parseSQLiteTime(createdAt)
	if err != nil {
		return nil, err
	}
	u.CreatedAt = ct
	return &u, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	var createdAt string
	if err := s.sql.QueryRowContext(ctx, `SELECT id, username, password_hash, created_at FROM users WHERE username=?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ct, err := parseSQLiteTime(createdAt)
	if err != nil {
		return nil, err
	}
	u.CreatedAt = ct
	return &u, nil
}

func (s *Store) CreateSession(ctx context.Context, userID string, token string, ttl time.Duration) (*Session, error) {
	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expires := now.Add(ttl)
	tokenHash := TokenHash(token)
	if _, err := s.sql.ExecContext(ctx, `INSERT INTO sessions(id, token_hash, user_id, expires_at, created_at, last_seen_at) VALUES(?,?,?,?,?,?)`,
		id, tokenHash, userID, expires.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	return s.GetSessionByToken(ctx, token)
}

func (s *Store) GetSessionByToken(ctx context.Context, token string) (*Session, error) {
	tokenHash := TokenHash(token)
	var sess Session
	var expiresAt, createdAt, lastSeen string
	if err := s.sql.QueryRowContext(ctx, `SELECT id, token_hash, user_id, expires_at, created_at, last_seen_at FROM sessions WHERE token_hash=?`, tokenHash).
		Scan(&sess.ID, &sess.TokenHash, &sess.UserID, &expiresAt, &createdAt, &lastSeen); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	sess.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return nil, err
	}
	sess.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, err
	}
	sess.LastSeenAt, err = time.Parse(time.RFC3339Nano, lastSeen)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) TouchSession(ctx context.Context, sessionID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.sql.ExecContext(ctx, `UPDATE sessions SET last_seen_at=? WHERE id=?`, now, sessionID)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.sql.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, sessionID)
	return err
}

type NewToken struct {
	TokenID string
	Name    string
	Token   string // plaintext, only return once
	Prefix  string
}

func (s *Store) CreateAPIToken(ctx context.Context, name string, plaintextToken string) (*APIToken, error) {
	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	prefix := TokenPrefix(plaintextToken)
	tokenHash := TokenHash(plaintextToken)
	if _, err := s.sql.ExecContext(ctx, `INSERT INTO api_tokens(id, name, prefix, token_hash, created_at) VALUES(?,?,?,?, datetime('now'))`,
		id, name, prefix, tokenHash); err != nil {
		return nil, err
	}
	return s.GetAPITokenByID(ctx, id)
}

func (s *Store) GetAPITokenByID(ctx context.Context, id string) (*APIToken, error) {
	var t APIToken
	var createdAt string
	var revokedAt sql.NullString
	if err := s.sql.QueryRowContext(ctx, `SELECT id, name, prefix, token_hash, created_at, revoked_at FROM api_tokens WHERE id=?`, id).
		Scan(&t.ID, &t.Name, &t.Prefix, &t.TokenHash, &createdAt, &revokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	tt, err := parseSQLiteTime(createdAt)
	if err != nil {
		return nil, err
	}
	t.CreatedAt = tt
	if revokedAt.Valid {
		rt, err := parseSQLiteTime(revokedAt.String)
		if err != nil {
			return nil, err
		}
		t.RevokedAt = &rt
	}
	return &t, nil
}

func (s *Store) ListAPITokens(ctx context.Context) ([]APIToken, error) {
	rows, err := s.sql.QueryContext(ctx, `SELECT id, name, prefix, token_hash, created_at, revoked_at FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		var createdAt string
		var revokedAt sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.TokenHash, &createdAt, &revokedAt); err != nil {
			return nil, err
		}
		ct, _ := parseSQLiteTime(createdAt)
		t.CreatedAt = ct
		if revokedAt.Valid {
			rt, _ := parseSQLiteTime(revokedAt.String)
			t.RevokedAt = &rt
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeAPIToken(ctx context.Context, id string) error {
	_, err := s.sql.ExecContext(ctx, `UPDATE api_tokens SET revoked_at=datetime('now') WHERE id=? AND revoked_at IS NULL`, id)
	return err
}

func (s *Store) FindAPITokenByPlaintext(ctx context.Context, token string) (*APIToken, error) {
	h := TokenHash(token)
	var t APIToken
	var createdAt string
	var revokedAt sql.NullString
	if err := s.sql.QueryRowContext(ctx, `SELECT id, name, prefix, token_hash, created_at, revoked_at FROM api_tokens WHERE token_hash=?`, h).
		Scan(&t.ID, &t.Name, &t.Prefix, &t.TokenHash, &createdAt, &revokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ct, _ := parseSQLiteTime(createdAt)
	t.CreatedAt = ct
	if revokedAt.Valid {
		rt, _ := parseSQLiteTime(revokedAt.String)
		t.RevokedAt = &rt
	}
	if t.RevokedAt != nil {
		return nil, ErrNotFound
	}
	return &t, nil
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	if err := s.sql.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return v, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.sql.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) EnsureDefaults(ctx context.Context) error {
	def := map[string]string{
		"base_domain":                   "example.com",
		"named_host_template":           "{app}-{service}-{env}.{base_domain}",
		"preview_host_template":         "pr-{app}-{repoSlug}-{pr}-{service}.{base_domain}",
		"docker_network":                "traefik",
		"traefik_acme_mode":             "tls",
		"traefik_alicloud_region_id":    "cn-hangzhou",
		"traefik_wildcard_enabled":      "1",
		"traefik_wildcard_include_apex": "0",
	}
	for k, v := range def {
		_, err := s.GetSetting(ctx, k)
		if err == nil {
			continue
		}
		if errors.Is(err, ErrNotFound) {
			if err := s.SetSetting(ctx, k, v); err != nil {
				return err
			}
			continue
		}
		return err
	}
	return nil
}

func (s *Store) CreateRepo(ctx context.Context, fullName string, webhookSecret string) (*Repo, error) {
	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	slug := RepoSlug(fullName)
	if _, err := s.sql.ExecContext(ctx, `INSERT INTO repos(id, full_name, slug, webhook_secret, created_at) VALUES(?,?,?,?, datetime('now'))`,
		id, fullName, slug, webhookSecret); err != nil {
		return nil, err
	}
	return s.GetRepoByID(ctx, id)
}

func (s *Store) UpdateRepoSecret(ctx context.Context, id string, secret string) error {
	_, err := s.sql.ExecContext(ctx, `UPDATE repos SET webhook_secret=? WHERE id=?`, secret, id)
	return err
}

func (s *Store) ListRepos(ctx context.Context) ([]Repo, error) {
	rows, err := s.sql.QueryContext(ctx, `SELECT id, full_name, slug, webhook_secret, created_at FROM repos ORDER BY full_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Repo
	for rows.Next() {
		var r Repo
		var createdAt string
		if err := rows.Scan(&r.ID, &r.FullName, &r.Slug, &r.WebhookSecret, &createdAt); err != nil {
			return nil, err
		}
		ct, _ := parseSQLiteTime(createdAt)
		r.CreatedAt = ct
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetRepoByID(ctx context.Context, id string) (*Repo, error) {
	var r Repo
	var createdAt string
	if err := s.sql.QueryRowContext(ctx, `SELECT id, full_name, slug, webhook_secret, created_at FROM repos WHERE id=?`, id).
		Scan(&r.ID, &r.FullName, &r.Slug, &r.WebhookSecret, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ct, _ := parseSQLiteTime(createdAt)
	r.CreatedAt = ct
	return &r, nil
}

func (s *Store) GetRepoByFullName(ctx context.Context, fullName string) (*Repo, error) {
	var r Repo
	var createdAt string
	if err := s.sql.QueryRowContext(ctx, `SELECT id, full_name, slug, webhook_secret, created_at FROM repos WHERE full_name=?`, fullName).
		Scan(&r.ID, &r.FullName, &r.Slug, &r.WebhookSecret, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ct, _ := parseSQLiteTime(createdAt)
	r.CreatedAt = ct
	return &r, nil
}

func (s *Store) DeleteRepo(ctx context.Context, id string) error {
	res, err := s.sql.ExecContext(ctx, `DELETE FROM repos WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func RepoSlug(fullName string) string {
	s := strings.ToLower(fullName)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	if out == "" {
		out = "repo"
	}
	return out
}

func (s *Store) CreateApp(ctx context.Context, appKey, name string) (*App, error) {
	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	if _, err := s.sql.ExecContext(ctx, `INSERT INTO apps(id, app_key, name, deploy_strategy, created_at) VALUES(?,?,?, 'recreate', datetime('now'))`, id, appKey, name); err != nil {
		return nil, err
	}
	return s.GetAppByID(ctx, id)
}

func (s *Store) ListApps(ctx context.Context) ([]App, error) {
	rows, err := s.sql.QueryContext(ctx, `SELECT id, app_key, name, deploy_strategy, created_at FROM apps ORDER BY app_key ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []App
	for rows.Next() {
		var a App
		var createdAt string
		if err := rows.Scan(&a.ID, &a.AppKey, &a.Name, &a.DeployStrategy, &createdAt); err != nil {
			return nil, err
		}
		ct, _ := parseSQLiteTime(createdAt)
		if strings.TrimSpace(a.DeployStrategy) == "" {
			a.DeployStrategy = "recreate"
		}
		a.CreatedAt = ct
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAppByID(ctx context.Context, id string) (*App, error) {
	var a App
	var createdAt string
	if err := s.sql.QueryRowContext(ctx, `SELECT id, app_key, name, deploy_strategy, created_at FROM apps WHERE id=?`, id).
		Scan(&a.ID, &a.AppKey, &a.Name, &a.DeployStrategy, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ct, _ := parseSQLiteTime(createdAt)
	if strings.TrimSpace(a.DeployStrategy) == "" {
		a.DeployStrategy = "recreate"
	}
	a.CreatedAt = ct
	return &a, nil
}

func (s *Store) GetAppByKey(ctx context.Context, appKey string) (*App, error) {
	var a App
	var createdAt string
	if err := s.sql.QueryRowContext(ctx, `SELECT id, app_key, name, deploy_strategy, created_at FROM apps WHERE app_key=?`, appKey).
		Scan(&a.ID, &a.AppKey, &a.Name, &a.DeployStrategy, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ct, _ := parseSQLiteTime(createdAt)
	if strings.TrimSpace(a.DeployStrategy) == "" {
		a.DeployStrategy = "recreate"
	}
	a.CreatedAt = ct
	return &a, nil
}

func (s *Store) UpdateAppDeployStrategy(ctx context.Context, id, strategy string) error {
	strategy = strings.TrimSpace(strings.ToLower(strategy))
	if strategy != "restart" {
		strategy = "recreate"
	}
	_, err := s.sql.ExecContext(ctx, `UPDATE apps SET deploy_strategy=? WHERE id=?`, strategy, id)
	return err
}

func (s *Store) DeleteApp(ctx context.Context, id string) error {
	_, err := s.sql.ExecContext(ctx, `DELETE FROM apps WHERE id=?`, id)
	return err
}

func (s *Store) CreateService(ctx context.Context, appID, serviceKey, name, image, command string, containerPort int, runUser string, env map[string]string, prodHost string) (*Service, error) {
	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	if containerPort == 0 {
		containerPort = 8080
	}
	if image == "" {
		image = "eclipse-temurin:17-jre"
	}
	if command == "" {
		command = "java -jar /app/app.jar"
	}
	if runUser == "" {
		runUser = "1000:1000"
	}
	entrypoints := "websecure"
	composeTemplate := strings.TrimSpace(`services:
  app:
    image: eclipse-temurin:17-jre
    command: sh -lc "java -jar /app/app.jar"
    volumes:
      {{- range $slotKey, $hostPath := .Artifacts }}
      - {{$hostPath}}:{{index $.SlotPaths $slotKey}}:ro
      {{- end }}
    labels:
      - traefik.enable=true
      - traefik.http.routers.{{.RouterName}}.rule=Host(` + "`{{.Host}}`" + `)
      - traefik.http.routers.{{.RouterName}}.entrypoints={{.EntryPoints}}
      - traefik.http.routers.{{.RouterName}}.tls=true
      - traefik.http.services.{{.TraefikService}}.loadbalancer.server.port={{.Port}}
    networks:
      - {{.Network}}
    restart: unless-stopped

networks:
  {{.Network}}:
    external: true
`)
	envJSON, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.sql.ExecContext(ctx, `INSERT INTO services(
		id, app_id, service_key, name, image, command, container_port, run_user, env_json, prod_host, traefik_entrypoints, compose_template, use_compose, enabled, created_at, updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, appID, serviceKey, name, image, command, containerPort, runUser, string(envJSON), prodHost, entrypoints, composeTemplate, 1, 1, now, now); err != nil {
		return nil, err
	}
	return s.GetServiceByID(ctx, id)
}

func (s *Store) UpdateService(ctx context.Context, serviceID string, patch Service) (*Service, error) {
	envJSON, err := json.Marshal(patch.Env)
	if err != nil {
		return nil, err
	}
	// Compose-only: keep use_compose always true.
	res, err := s.sql.ExecContext(ctx, `UPDATE services
		SET name=?, image=?, command=?, container_port=?, run_user=?, env_json=?, prod_host=?, traefik_entrypoints=?, compose_template=?, use_compose=1, enabled=?, revision=revision+1, updated_at=datetime('now')
		WHERE id=?`,
		patch.Name, patch.Image, patch.Command, patch.ContainerPort, patch.RunUser, string(envJSON), patch.ProdHost, patch.TraefikEntrypnts, patch.ComposeTemplate, boolToInt(patch.Enabled), serviceID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return s.GetServiceByID(ctx, serviceID)
}

func (s *Store) GetServiceByID(ctx context.Context, id string) (*Service, error) {
	var svc Service
	var envJSON string
	var createdAt, updatedAt string
	var enabled, useCompose int
	if err := s.sql.QueryRowContext(ctx, `SELECT id, app_id, service_key, name, image, command, container_port, run_user, env_json, prod_host, traefik_entrypoints, compose_template, use_compose, revision, enabled, created_at, updated_at
		FROM services WHERE id=?`, id).
		Scan(&svc.ID, &svc.AppID, &svc.ServiceKey, &svc.Name, &svc.Image, &svc.Command, &svc.ContainerPort, &svc.RunUser, &envJSON, &svc.ProdHost, &svc.TraefikEntrypnts, &svc.ComposeTemplate, &useCompose, &svc.Revision, &enabled, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(envJSON), &svc.Env)
	svc.Enabled = enabled != 0
	svc.UseCompose = useCompose != 0
	svc.CreatedAt, _ = parseSQLiteTime(createdAt)
	svc.UpdatedAt, _ = parseSQLiteTime(updatedAt)
	return &svc, nil
}

func (s *Store) GetServiceByKey(ctx context.Context, appID, serviceKey string) (*Service, error) {
	var id string
	if err := s.sql.QueryRowContext(ctx, `SELECT id FROM services WHERE app_id=? AND service_key=?`, appID, serviceKey).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.GetServiceByID(ctx, id)
}

func (s *Store) ListServicesByApp(ctx context.Context, appID string) ([]Service, error) {
	rows, err := s.sql.QueryContext(ctx, `SELECT id, app_id, service_key, name, image, command, container_port, run_user, env_json, prod_host, traefik_entrypoints, compose_template, use_compose, revision, enabled, created_at, updated_at
		FROM services WHERE app_id=? ORDER BY service_key ASC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Service
	for rows.Next() {
		var svc Service
		var envJSON string
		var createdAt, updatedAt string
		var enabled, useCompose int
		if err := rows.Scan(&svc.ID, &svc.AppID, &svc.ServiceKey, &svc.Name, &svc.Image, &svc.Command, &svc.ContainerPort, &svc.RunUser,
			&envJSON, &svc.ProdHost, &svc.TraefikEntrypnts, &svc.ComposeTemplate, &useCompose, &svc.Revision, &enabled, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(envJSON), &svc.Env)
		svc.Enabled = enabled != 0
		svc.UseCompose = useCompose != 0
		svc.CreatedAt, _ = parseSQLiteTime(createdAt)
		svc.UpdatedAt, _ = parseSQLiteTime(updatedAt)
		out = append(out, svc)
	}
	return out, rows.Err()
}

func (s *Store) DeleteService(ctx context.Context, id string) error {
	_, err := s.sql.ExecContext(ctx, `DELETE FROM services WHERE id=?`, id)
	return err
}

func (s *Store) CreateSlot(ctx context.Context, serviceID, slotKey, name, repoID, containerPath string) (*Slot, error) {
	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	if containerPath == "" {
		return nil, fmt.Errorf("container_path required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.sql.ExecContext(ctx, `INSERT INTO slots(id, service_id, slot_key, name, repo_id, container_path, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		id, serviceID, slotKey, name, repoID, containerPath, now, now); err != nil {
		return nil, err
	}
	return s.GetSlotByID(ctx, id)
}

func (s *Store) UpdateSlot(ctx context.Context, slotID string, patch Slot) (*Slot, error) {
	res, err := s.sql.ExecContext(ctx, `UPDATE slots SET name=?, repo_id=?, container_path=?, updated_at=datetime('now') WHERE id=?`,
		patch.Name, patch.RepoID, patch.ContainerPath, slotID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return s.GetSlotByID(ctx, slotID)
}

func (s *Store) GetSlotByID(ctx context.Context, id string) (*Slot, error) {
	var sl Slot
	var createdAt, updatedAt string
	if err := s.sql.QueryRowContext(ctx, `SELECT id, service_id, slot_key, name, repo_id, container_path, created_at, updated_at FROM slots WHERE id=?`, id).
		Scan(&sl.ID, &sl.ServiceID, &sl.SlotKey, &sl.Name, &sl.RepoID, &sl.ContainerPath, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	sl.CreatedAt, _ = parseSQLiteTime(createdAt)
	sl.UpdatedAt, _ = parseSQLiteTime(updatedAt)
	return &sl, nil
}

func (s *Store) GetSlotByKey(ctx context.Context, serviceID, slotKey string) (*Slot, error) {
	var id string
	if err := s.sql.QueryRowContext(ctx, `SELECT id FROM slots WHERE service_id=? AND slot_key=?`, serviceID, slotKey).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.GetSlotByID(ctx, id)
}

func (s *Store) ListSlotsByService(ctx context.Context, serviceID string) ([]Slot, error) {
	rows, err := s.sql.QueryContext(ctx, `SELECT id, service_id, slot_key, name, repo_id, container_path, created_at, updated_at
		FROM slots WHERE service_id=? ORDER BY slot_key ASC`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Slot
	for rows.Next() {
		var sl Slot
		var createdAt, updatedAt string
		if err := rows.Scan(&sl.ID, &sl.ServiceID, &sl.SlotKey, &sl.Name, &sl.RepoID, &sl.ContainerPath, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		sl.CreatedAt, _ = parseSQLiteTime(createdAt)
		sl.UpdatedAt, _ = parseSQLiteTime(updatedAt)
		out = append(out, sl)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSlot(ctx context.Context, id string) error {
	_, err := s.sql.ExecContext(ctx, `DELETE FROM slots WHERE id=?`, id)
	return err
}

func (s *Store) CreateNamedEnv(ctx context.Context, appID, name string) (*Env, error) {
	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	if _, err := s.sql.ExecContext(ctx, `INSERT INTO envs(id, app_id, kind, name, created_at) VALUES(?,?, 'named', ?, datetime('now'))`, id, appID, name); err != nil {
		return nil, err
	}
	return s.GetEnvByID(ctx, id)
}

func (s *Store) EnsureNamedEnv(ctx context.Context, appID, name string) (*Env, error) {
	// The schema's UNIQUE constraint includes nullable columns (repo_id/pr_number),
	// so we must enforce uniqueness at the application layer.
	if id, err := s.GetEnvIDByName(ctx, appID, name); err == nil {
		return s.GetEnvByID(ctx, id)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return s.CreateNamedEnv(ctx, appID, name)
}

func (s *Store) FindPreviewPlaceholderEnvID(ctx context.Context, appID string) (string, error) {
	var id string
	if err := s.sql.QueryRowContext(ctx, `SELECT id FROM envs
		WHERE app_id=? AND kind='preview' AND repo_id IS NULL AND pr_number IS NULL AND deleted_at IS NULL
		ORDER BY created_at ASC LIMIT 1`, appID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return id, nil
}

func (s *Store) CreatePreviewPlaceholderEnv(ctx context.Context, appID string) (*Env, error) {
	id, err := ids.New()
	if err != nil {
		return nil, err
	}
	// Placeholder "preview" env (without repo/pr). Real preview envs are created
	// on demand per PR via UpsertPreviewEnv.
	if _, err := s.sql.ExecContext(ctx, `INSERT INTO envs(id, app_id, kind, name, created_at)
		VALUES(?,?, 'preview', 'preview', datetime('now'))`, id, appID); err != nil {
		return nil, err
	}
	return s.GetEnvByID(ctx, id)
}

func (s *Store) EnsurePreviewPlaceholderEnv(ctx context.Context, appID string) (*Env, error) {
	if id, err := s.FindPreviewPlaceholderEnvID(ctx, appID); err == nil {
		return s.GetEnvByID(ctx, id)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return s.CreatePreviewPlaceholderEnv(ctx, appID)
}

func (s *Store) UpsertPreviewEnv(ctx context.Context, appID string, repo Repo, prNumber int) (*Env, error) {
	var id string
	err := s.sql.QueryRowContext(ctx, `SELECT id FROM envs WHERE app_id=? AND kind='preview' AND repo_id=? AND pr_number=? AND deleted_at IS NULL`,
		appID, repo.ID, prNumber).Scan(&id)
	if err == nil {
		return s.GetEnvByID(ctx, id)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Preview template: a named env called "preview". New PR preview envs inherit
	// its current_snapshot_id so that config/artifacts can be shared by default.
	var templateSnapshot sql.NullString
	_ = s.sql.QueryRowContext(ctx, `SELECT current_snapshot_id FROM envs
		WHERE app_id=? AND kind='named' AND name='preview' AND deleted_at IS NULL
		ORDER BY created_at ASC LIMIT 1`, appID).Scan(&templateSnapshot)

	id, err = ids.New()
	if err != nil {
		return nil, err
	}
	if _, err := s.sql.ExecContext(ctx, `INSERT INTO envs(id, app_id, kind, name, repo_id, pr_number, current_snapshot_id, created_at)
		VALUES(?,?, 'preview', 'preview', ?, ?, ?, datetime('now'))`,
		id, appID, repo.ID, prNumber, nullableString(nullIfEmpty(templateSnapshot.String))); err != nil {
		return nil, err
	}
	return s.GetEnvByID(ctx, id)
}

func nullIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func (s *Store) ListEnvsByApp(ctx context.Context, appID string) ([]Env, error) {
	rows, err := s.sql.QueryContext(ctx, `SELECT e.id, e.app_id, e.kind, e.name, e.repo_id, e.pr_number, e.current_snapshot_id, e.created_at, e.deleted_at,
		r.full_name, r.slug
		FROM envs e
		LEFT JOIN repos r ON e.repo_id=r.id
		WHERE e.app_id=? AND e.deleted_at IS NULL
		ORDER BY e.kind ASC, e.name ASC, e.pr_number ASC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Env
	for rows.Next() {
		var e Env
		var repoID sql.NullString
		var pr sql.NullInt64
		var cur sql.NullString
		var createdAt string
		var deletedAt sql.NullString
		var repoFull sql.NullString
		var repoSlug sql.NullString
		if err := rows.Scan(&e.ID, &e.AppID, &e.Kind, &e.Name, &repoID, &pr, &cur, &createdAt, &deletedAt, &repoFull, &repoSlug); err != nil {
			return nil, err
		}
		if repoID.Valid {
			e.RepoID = &repoID.String
		}
		if pr.Valid {
			pp := int(pr.Int64)
			e.PRNumber = &pp
		}
		if cur.Valid {
			e.CurrentSnapshot = &cur.String
		}
		ct, _ := parseSQLiteTime(createdAt)
		e.CreatedAt = ct
		if deletedAt.Valid {
			dt, _ := parseSQLiteTime(deletedAt.String)
			e.DeletedAt = &dt
		}
		if repoFull.Valid {
			e.RepoFullName = &repoFull.String
		}
		if repoSlug.Valid {
			e.RepoSlug = &repoSlug.String
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetEnvByID(ctx context.Context, id string) (*Env, error) {
	var e Env
	var repoID sql.NullString
	var pr sql.NullInt64
	var cur sql.NullString
	var createdAt string
	var deletedAt sql.NullString
	var repoFull sql.NullString
	var repoSlug sql.NullString
	if err := s.sql.QueryRowContext(ctx, `SELECT e.id, e.app_id, e.kind, e.name, e.repo_id, e.pr_number, e.current_snapshot_id, e.created_at, e.deleted_at,
		r.full_name, r.slug
		FROM envs e
		LEFT JOIN repos r ON e.repo_id=r.id
		WHERE e.id=?`, id).
		Scan(&e.ID, &e.AppID, &e.Kind, &e.Name, &repoID, &pr, &cur, &createdAt, &deletedAt, &repoFull, &repoSlug); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if repoID.Valid {
		e.RepoID = &repoID.String
	}
	if pr.Valid {
		pp := int(pr.Int64)
		e.PRNumber = &pp
	}
	if cur.Valid {
		e.CurrentSnapshot = &cur.String
	}
	ct, _ := parseSQLiteTime(createdAt)
	e.CreatedAt = ct
	if deletedAt.Valid {
		dt, _ := parseSQLiteTime(deletedAt.String)
		e.DeletedAt = &dt
	}
	if repoFull.Valid {
		e.RepoFullName = &repoFull.String
	}
	if repoSlug.Valid {
		e.RepoSlug = &repoSlug.String
	}
	return &e, nil
}

func (s *Store) SoftDeleteEnv(ctx context.Context, envID string) error {
	_, err := s.sql.ExecContext(ctx, `UPDATE envs SET deleted_at=datetime('now') WHERE id=? AND deleted_at IS NULL`, envID)
	return err
}

type UploadParams struct {
	ArtifactID string
	AppID      string
	ServiceID  string
	SlotID     string
	RepoID     string
	EnvID      string
	SHA        string
	Ref        string
	PRNumber   *int
	Filename   string
	SizeBytes  int64
	SHA256Hex  string
	StoredPath string
	TokenID    *string
	UserID     *string
	Note       string
}

type UploadBatchEntry struct {
	ArtifactID string
	SlotID     string
	RepoID     string
	SHA        string
	Ref        string
	PRNumber   *int
	Filename   string
	SizeBytes  int64
	SHA256Hex  string
	StoredPath string
}

type UploadBatchParams struct {
	AppID     string
	ServiceID string
	EnvID     string
	Entries   []UploadBatchEntry
	TokenID   *string
	UserID    *string
	Note      string
}

type UploadBatchResult struct {
	SnapshotID  string
	EnvID       string
	PrevSnapID  *string
	ArtifactIDs []string
}

type UploadResult struct {
	ArtifactID  string
	SnapshotID  string
	EnvID       string
	PrevSnapID  *string
	NowSnapID   string
	SHA256Hex   string
	StoredPath  string
	ArtifactHex string
}

func (s *Store) CreateArtifactAndSnapshot(ctx context.Context, p UploadParams) (*UploadResult, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	artifactID := strings.TrimSpace(p.ArtifactID)
	if artifactID == "" {
		return nil, fmt.Errorf("artifact_id required")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO artifacts(
		id, app_id, service_id, slot_id, repo_id, sha, ref, pr_number, original_filename, size_bytes, sha256_hex, stored_path, created_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?, datetime('now'))`,
		artifactID, p.AppID, p.ServiceID, p.SlotID, p.RepoID, p.SHA, p.Ref, nullableInt(p.PRNumber), p.Filename, p.SizeBytes, p.SHA256Hex, p.StoredPath); err != nil {
		return nil, err
	}

	var prevSnapshot sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT current_snapshot_id FROM envs WHERE id=?`, p.EnvID).Scan(&prevSnapshot); err != nil {
		return nil, err
	}

	snapshotID, err := ids.New()
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO snapshots(id, env_id, created_at, created_by_user_id, created_by_token_id, note)
		VALUES(?, ?, datetime('now'), ?, ?, ?)`, snapshotID, p.EnvID, nullableString(p.UserID), nullableString(p.TokenID), p.Note); err != nil {
		return nil, err
	}

	if prevSnapshot.Valid {
		if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot_slots(snapshot_id, slot_id, artifact_id)
			SELECT ?, slot_id, artifact_id FROM snapshot_slots WHERE snapshot_id=?`, snapshotID, prevSnapshot.String); err != nil {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot_slots(snapshot_id, slot_id, artifact_id)
		VALUES(?,?,?)
		ON CONFLICT(snapshot_id, slot_id) DO UPDATE SET artifact_id=excluded.artifact_id`, snapshotID, p.SlotID, artifactID); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE envs SET current_snapshot_id=? WHERE id=?`, snapshotID, p.EnvID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	var prev *string
	if prevSnapshot.Valid {
		prev = &prevSnapshot.String
	}
	return &UploadResult{
		ArtifactID: artifactID,
		SnapshotID: snapshotID,
		EnvID:      p.EnvID,
		PrevSnapID: prev,
		NowSnapID:  snapshotID,
		SHA256Hex:  p.SHA256Hex,
		StoredPath: p.StoredPath,
	}, nil
}

func (s *Store) CreateArtifactsAndSnapshotBatch(ctx context.Context, p UploadBatchParams) (*UploadBatchResult, error) {
	if len(p.Entries) == 0 {
		return nil, fmt.Errorf("entries required")
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for _, e := range p.Entries {
		artifactID := strings.TrimSpace(e.ArtifactID)
		if artifactID == "" {
			return nil, fmt.Errorf("artifact_id required")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO artifacts(
			id, app_id, service_id, slot_id, repo_id, sha, ref, pr_number, original_filename, size_bytes, sha256_hex, stored_path, created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?, datetime('now'))`,
			artifactID, p.AppID, p.ServiceID, e.SlotID, e.RepoID, e.SHA, e.Ref, nullableInt(e.PRNumber), e.Filename, e.SizeBytes, e.SHA256Hex, e.StoredPath); err != nil {
			return nil, err
		}
	}

	var prevSnapshot sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT current_snapshot_id FROM envs WHERE id=?`, p.EnvID).Scan(&prevSnapshot); err != nil {
		return nil, err
	}

	snapshotID, err := ids.New()
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO snapshots(id, env_id, created_at, created_by_user_id, created_by_token_id, note)
		VALUES(?, ?, datetime('now'), ?, ?, ?)`, snapshotID, p.EnvID, nullableString(p.UserID), nullableString(p.TokenID), p.Note); err != nil {
		return nil, err
	}

	if prevSnapshot.Valid {
		if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot_slots(snapshot_id, slot_id, artifact_id)
			SELECT ?, slot_id, artifact_id FROM snapshot_slots WHERE snapshot_id=?`, snapshotID, prevSnapshot.String); err != nil {
			return nil, err
		}
	}

	for _, e := range p.Entries {
		if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot_slots(snapshot_id, slot_id, artifact_id)
			VALUES(?,?,?)
			ON CONFLICT(snapshot_id, slot_id) DO UPDATE SET artifact_id=excluded.artifact_id`, snapshotID, e.SlotID, e.ArtifactID); err != nil {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE envs SET current_snapshot_id=? WHERE id=?`, snapshotID, p.EnvID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	var prev *string
	if prevSnapshot.Valid {
		prev = &prevSnapshot.String
	}

	artifactIDs := make([]string, 0, len(p.Entries))
	for _, e := range p.Entries {
		artifactIDs = append(artifactIDs, e.ArtifactID)
	}

	return &UploadBatchResult{
		SnapshotID:  snapshotID,
		EnvID:       p.EnvID,
		PrevSnapID:  prev,
		ArtifactIDs: artifactIDs,
	}, nil
}

func (s *Store) ListSnapshots(ctx context.Context, envID string) ([]Snapshot, error) {
	rows, err := s.sql.QueryContext(ctx, `SELECT id, env_id, created_at, created_by_user_id, created_by_token_id, note
		FROM snapshots WHERE env_id=? ORDER BY created_at DESC`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		var sn Snapshot
		var createdAt string
		var u sql.NullString
		var t sql.NullString
		if err := rows.Scan(&sn.ID, &sn.EnvID, &createdAt, &u, &t, &sn.Note); err != nil {
			return nil, err
		}
		ct, _ := parseSQLiteTime(createdAt)
		sn.CreatedAt = ct
		if u.Valid {
			sn.CreatedByUserID = &u.String
		}
		if t.Valid {
			sn.CreatedByToken = &t.String
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

func (s *Store) GetSnapshotSlotArtifacts(ctx context.Context, snapshotID, serviceID string) (map[string]Artifact, error) {
	// map slot_key -> artifact
	rows, err := s.sql.QueryContext(ctx, `SELECT sl.slot_key, a.id, a.app_id, a.service_id, a.slot_id, a.repo_id, a.sha, a.ref, a.pr_number,
		a.original_filename, a.size_bytes, a.sha256_hex, a.stored_path, a.created_at
		FROM snapshot_slots ss
		JOIN slots sl ON ss.slot_id = sl.id
		JOIN artifacts a ON ss.artifact_id = a.id
		WHERE ss.snapshot_id=? AND sl.service_id=?`, snapshotID, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]Artifact)
	for rows.Next() {
		var slotKey string
		var a Artifact
		var createdAt string
		var pr sql.NullInt64
		if err := rows.Scan(&slotKey, &a.ID, &a.AppID, &a.ServiceID, &a.SlotID, &a.RepoID, &a.SHA, &a.Ref, &pr,
			&a.OriginalFilename, &a.SizeBytes, &a.SHA256Hex, &a.StoredPath, &createdAt); err != nil {
			return nil, err
		}
		if pr.Valid {
			pp := int(pr.Int64)
			a.PRNumber = &pp
		}
		ct, _ := parseSQLiteTime(createdAt)
		a.CreatedAt = ct
		out[slotKey] = a
	}
	return out, rows.Err()
}

func parseSQLiteTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unparseable time: %q", s)
}

func (s *Store) SetEnvCurrentSnapshot(ctx context.Context, envID, snapshotID string) error {
	_, err := s.sql.ExecContext(ctx, `UPDATE envs SET current_snapshot_id=? WHERE id=?`, snapshotID, envID)
	return err
}

func (s *Store) GetEnvCurrentSnapshotID(ctx context.Context, envID string) (*string, error) {
	var cur sql.NullString
	if err := s.sql.QueryRowContext(ctx, `SELECT current_snapshot_id FROM envs WHERE id=?`, envID).Scan(&cur); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !cur.Valid {
		return nil, nil
	}
	return &cur.String, nil
}

func (s *Store) GetEnvIDByName(ctx context.Context, appID, envName string) (string, error) {
	var id string
	if err := s.sql.QueryRowContext(ctx, `SELECT id FROM envs WHERE app_id=? AND kind='named' AND name=? AND deleted_at IS NULL`, appID, envName).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return id, nil
}

func (s *Store) FindPreviewEnvID(ctx context.Context, appID, repoID string, prNumber int) (string, error) {
	var id string
	if err := s.sql.QueryRowContext(ctx, `SELECT id FROM envs WHERE app_id=? AND kind='preview' AND repo_id=? AND pr_number=? AND deleted_at IS NULL`,
		appID, repoID, prNumber).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return id, nil
}

func (s *Store) FindEnvsForRepoPR(ctx context.Context, repoID string, prNumber int) ([]Env, error) {
	rows, err := s.sql.QueryContext(ctx, `SELECT e.id, e.app_id, e.kind, e.name, e.repo_id, e.pr_number, e.current_snapshot_id, e.created_at, e.deleted_at,
		r.full_name, r.slug
		FROM envs e
		LEFT JOIN repos r ON e.repo_id=r.id
		WHERE e.kind='preview' AND e.repo_id=? AND e.pr_number=? AND e.deleted_at IS NULL
		ORDER BY e.created_at DESC`, repoID, prNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Env
	for rows.Next() {
		var e Env
		var repoIDNull sql.NullString
		var pr sql.NullInt64
		var cur sql.NullString
		var createdAt string
		var deletedAt sql.NullString
		var repoFull sql.NullString
		var repoSlug sql.NullString
		if err := rows.Scan(&e.ID, &e.AppID, &e.Kind, &e.Name, &repoIDNull, &pr, &cur, &createdAt, &deletedAt, &repoFull, &repoSlug); err != nil {
			return nil, err
		}
		if repoIDNull.Valid {
			e.RepoID = &repoIDNull.String
		}
		if pr.Valid {
			pp := int(pr.Int64)
			e.PRNumber = &pp
		}
		if cur.Valid {
			e.CurrentSnapshot = &cur.String
		}
		e.CreatedAt, _ = parseSQLiteTime(createdAt)
		if deletedAt.Valid {
			dt, _ := parseSQLiteTime(deletedAt.String)
			e.DeletedAt = &dt
		}
		if repoFull.Valid {
			e.RepoFullName = &repoFull.String
		}
		if repoSlug.Valid {
			e.RepoSlug = &repoSlug.String
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ArtifactHexDigest(sha256Hex string) (string, error) {
	_, err := hex.DecodeString(sha256Hex)
	if err != nil {
		return "", err
	}
	return sha256Hex, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
