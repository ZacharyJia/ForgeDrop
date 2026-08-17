package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

type Client struct {
	baseURL string
	http    *http.Client
	token   string
}

type SetupStatus struct {
	Allowed   bool `json:"allowed"`
	UserCount int  `json:"user_count"`
}

type Repo struct {
	ID            string `json:"id"`
	FullName      string `json:"full_name"`
	Slug          string `json:"slug"`
	WebhookSecret string `json:"webhook_secret"`
}

type App struct {
	ID     string `json:"id"`
	AppKey string `json:"app_key"`
	Name   string `json:"name"`
}

type Env struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	RepoID       *string `json:"repo_id"`
	PRNumber     *int    `json:"pr_number"`
	ChangeSet    *string `json:"change_set"`
	RepoFullName *string `json:"repo_full_name"`
}

type Service struct {
	ID                 string            `json:"id"`
	AppID              string            `json:"app_id"`
	ServiceKey         string            `json:"service_key"`
	Name               string            `json:"name"`
	Image              string            `json:"image"`
	Command            string            `json:"command"`
	ContainerPort      int               `json:"container_port"`
	RunUser            string            `json:"run_user"`
	Env                map[string]string `json:"env"`
	ProdHost           string            `json:"prod_host"`
	TraefikEntrypoints string            `json:"traefik_entrypoints"`
	ComposeTemplate    string            `json:"compose_template"`
	DeployStrategy     string            `json:"deploy_strategy"`
	Enabled            bool              `json:"enabled"`
}

type Slot struct {
	ID            string   `json:"id"`
	ServiceID     string   `json:"service_id"`
	SlotKey       string   `json:"slot_key"`
	Name          string   `json:"name"`
	RepoIDs       []string `json:"repo_ids"`
	ContainerPath string   `json:"container_path"`
	MountType     string   `json:"mount_type"`
}

type Token struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Scope  string `json:"scope"`
	Prefix string `json:"prefix"`
}

type CreatedToken struct {
	Token      Token  `json:"token"`
	PlainToken string `json:"plain_token"`
}

type PublicSkillBundle struct {
	Name  string            `json:"name"`
	Files []PublicSkillFile `json:"files"`
}

type PublicSkillFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

var adminSettingKeys = []string{
	"base_domain",
	"named_host_template",
	"preview_host_template",
	"docker_network",
	"traefik_acme_email",
	"traefik_acme_mode",
	"traefik_alicloud_region_id",
	"traefik_wildcard_enabled",
	"traefik_wildcard_include_apex",
}

type ServiceCreateRequest struct {
	ServiceKey    string            `json:"service_key"`
	Name          string            `json:"name"`
	Image         string            `json:"image,omitempty"`
	Command       string            `json:"command,omitempty"`
	ContainerPort int               `json:"container_port,omitempty"`
	RunUser       string            `json:"run_user,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	ProdHost      string            `json:"prod_host,omitempty"`
}

type ServiceUpdateRequest struct {
	Name               string            `json:"name"`
	ContainerPort      int               `json:"container_port"`
	Env                map[string]string `json:"env"`
	ProdHost           string            `json:"prod_host"`
	TraefikEntrypoints string            `json:"traefik_entrypoints"`
	ComposeTemplate    string            `json:"compose_template"`
	DeployStrategy     string            `json:"deploy_strategy"`
	Enabled            bool              `json:"enabled"`
}

type SlotUpsertRequest struct {
	SlotKey       string   `json:"slot_key,omitempty"`
	Name          string   `json:"name"`
	RepoIDs       []string `json:"repo_ids"`
	MountType     string   `json:"mount_type"`
	ContainerPath string   `json:"container_path"`
}

func NewClient(baseURL string) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		http: &http.Client{
			Jar: jar,
		},
	}, nil
}

func (c *Client) SetBearerToken(token string) {
	c.token = strings.TrimSpace(token)
}

func (c *Client) SetupStatus(ctx context.Context) (*SetupStatus, error) {
	var out SetupStatus
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/setup/status", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Setup(ctx context.Context, username, password string) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/setup", map[string]string{
		"username": username,
		"password": password,
	}, nil)
}

func (c *Client) Login(ctx context.Context, username, password string) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, nil)
}

func (c *Client) AdminMe(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodGet, "/api/v1/admin/me", nil, &map[string]any{})
}

func (c *Client) GetSettings(ctx context.Context) (map[string]string, error) {
	var raw map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/admin/settings", nil, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(adminSettingKeys))
	for _, key := range adminSettingKeys {
		v, ok := raw[key]
		if !ok || v == nil {
			out[key] = ""
			continue
		}
		switch val := v.(type) {
		case string:
			out[key] = val
		case bool:
			if val {
				out[key] = "true"
			} else {
				out[key] = "false"
			}
		default:
			out[key] = fmt.Sprint(val)
		}
	}
	return out, nil
}

func (c *Client) UpdateSettings(ctx context.Context, settings map[string]string) error {
	return c.doJSON(ctx, http.MethodPut, "/api/v1/admin/settings", settings, nil)
}

func (c *Client) ListRepos(ctx context.Context) ([]Repo, error) {
	var out []Repo
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/admin/repos", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateRepo(ctx context.Context, fullName, webhookSecret string) (*Repo, error) {
	var out Repo
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/admin/repos", map[string]string{
		"full_name":      fullName,
		"webhook_secret": webhookSecret,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateRepoSecret(ctx context.Context, repoID, webhookSecret string) error {
	return c.doJSON(ctx, http.MethodPut, "/api/v1/admin/repos/"+repoID, map[string]string{
		"webhook_secret": webhookSecret,
	}, nil)
}

func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	var out []App
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/admin/apps", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateApp(ctx context.Context, appKey, name string) (*App, error) {
	var out App
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/admin/apps", map[string]string{
		"app_key": appKey,
		"name":    name,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateAppName(ctx context.Context, appID, name string) (*App, error) {
	var out App
	if err := c.doJSON(ctx, http.MethodPut, "/api/v1/admin/apps/"+appID, map[string]string{
		"name": name,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListEnvs(ctx context.Context, appID string) ([]Env, error) {
	var out []Env
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/admin/apps/"+appID+"/envs", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateEnv(ctx context.Context, appID, name string) (*Env, error) {
	var out Env
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/admin/apps/"+appID+"/envs", map[string]string{
		"name": name,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) StopEnv(ctx context.Context, envID string) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/admin/envs/"+envID+"/stop", nil, nil)
}

func (c *Client) DeleteEnv(ctx context.Context, envID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/api/v1/admin/envs/"+envID, nil, nil)
}

func (c *Client) ListServices(ctx context.Context, appID string) ([]Service, error) {
	var out []Service
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/admin/apps/"+appID+"/services", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateService(ctx context.Context, appID string, req ServiceCreateRequest) (*Service, error) {
	var out Service
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/admin/apps/"+appID+"/services", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateService(ctx context.Context, serviceID string, req ServiceUpdateRequest) (*Service, error) {
	var out Service
	if err := c.doJSON(ctx, http.MethodPut, "/api/v1/admin/services/"+serviceID, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListSlots(ctx context.Context, serviceID string) ([]Slot, error) {
	var out []Slot
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/admin/services/"+serviceID+"/slots", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateSlot(ctx context.Context, serviceID string, req SlotUpsertRequest) (*Slot, error) {
	var out Slot
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/admin/services/"+serviceID+"/slots", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateSlot(ctx context.Context, serviceID, slotID string, req SlotUpsertRequest) (*Slot, error) {
	req.SlotKey = ""
	var out Slot
	if err := c.doJSON(ctx, http.MethodPut, "/api/v1/admin/services/"+serviceID+"/slots/"+slotID, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListTokens(ctx context.Context) ([]Token, error) {
	var out []Token
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/admin/tokens", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateToken(ctx context.Context, name, scope string) (*CreatedToken, error) {
	var out CreatedToken
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/admin/tokens", map[string]string{
		"name":  name,
		"scope": scope,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RevokeToken(ctx context.Context, tokenID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/api/v1/admin/tokens/"+tokenID, nil, nil)
}

func (c *Client) ListPublicSkills(ctx context.Context) ([]PublicSkillBundle, error) {
	var out struct {
		Skills []PublicSkillBundle `json:"skills"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/agents/skill", nil, &out); err != nil {
		return nil, err
	}
	return out.Skills, nil
}

func (c *Client) GetPublicSkill(ctx context.Context, name string) (*PublicSkillBundle, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("skill name is required")
	}
	var out PublicSkillBundle
	if err := c.doJSON(ctx, http.MethodGet, "/agents/skill/"+url.PathEscape(name), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("%s %s failed: %s", method, path, msg)
	}

	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}
