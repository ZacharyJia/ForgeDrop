package bootstrap

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

type ApplyOptions struct {
	Username string
	Password string
}

type ApplyResult struct {
	Repos    []ApplyRepoResult    `json:"repos"`
	App      ApplyAppResult       `json:"app"`
	Services []ApplyServiceResult `json:"services"`
	APIToken *ApplyTokenResult    `json:"api_token,omitempty"`
}

type ApplyRepoResult struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
	Action   string `json:"action"`
}

type ApplyAppResult struct {
	ID       string            `json:"id"`
	AppKey   string            `json:"app_key"`
	Name     string            `json:"name"`
	Action   string            `json:"action"`
	Envs     []ApplyEnvResult  `json:"envs"`
	Settings map[string]string `json:"settings,omitempty"`
}

type ApplyEnvResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Action string `json:"action"`
}

type ApplyServiceResult struct {
	ID      string            `json:"id"`
	Key     string            `json:"service_key"`
	Name    string            `json:"name"`
	Action  string            `json:"action"`
	Slots   []ApplySlotResult `json:"slots"`
	Enabled bool              `json:"enabled"`
}

type ApplySlotResult struct {
	ID        string   `json:"id"`
	SlotKey   string   `json:"slot_key"`
	Action    string   `json:"action"`
	RepoIDs   []string `json:"repo_ids"`
	MountType string   `json:"mount_type"`
}

type ApplyTokenResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Action     string `json:"action"`
	PlainToken string `json:"plain_token,omitempty"`
}

func Apply(ctx context.Context, client *Client, manifest *Manifest, opts ApplyOptions) (*ApplyResult, error) {
	if manifest == nil {
		return nil, fmt.Errorf("manifest is required")
	}
	if err := ensureAuthenticated(ctx, client, opts); err != nil {
		return nil, err
	}
	if len(manifest.Settings) > 0 {
		if err := client.UpdateSettings(ctx, manifest.Settings); err != nil {
			return nil, err
		}
	}

	result := &ApplyResult{
		App: ApplyAppResult{
			Settings: manifest.Settings,
		},
	}

	repos, err := ensureRepos(ctx, client, manifest.Repos)
	if err != nil {
		return nil, err
	}
	result.Repos = repos.Results

	app, appAction, err := ensureApp(ctx, client, manifest.App)
	if err != nil {
		return nil, err
	}
	result.App.ID = app.ID
	result.App.AppKey = app.AppKey
	result.App.Name = app.Name
	result.App.Action = appAction

	envs, err := ensureEnvs(ctx, client, app.ID, manifest.App.DesiredEnvNames())
	if err != nil {
		return nil, err
	}
	result.App.Envs = envs

	services, err := ensureServices(ctx, client, app.ID, manifest.App.Services, repos.ByFullName)
	if err != nil {
		return nil, err
	}
	result.Services = services

	if manifest.APIToken != nil {
		token, err := ensureToken(ctx, client, *manifest.APIToken)
		if err != nil {
			return nil, err
		}
		result.APIToken = token
	}

	return result, nil
}

func ensureAuthenticated(ctx context.Context, client *Client, opts ApplyOptions) error {
	opts.Username = strings.TrimSpace(opts.Username)
	if opts.Username == "" || opts.Password == "" {
		return fmt.Errorf("username and password are required")
	}

	status, err := client.SetupStatus(ctx)
	if err != nil {
		return err
	}
	if status.Allowed {
		if err := client.Setup(ctx, opts.Username, opts.Password); err != nil {
			return fmt.Errorf("initial setup failed: %w", err)
		}
	}
	if err := client.Login(ctx, opts.Username, opts.Password); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	return nil
}

type ensuredRepos struct {
	ByFullName map[string]Repo
	Results    []ApplyRepoResult
}

func ensureRepos(ctx context.Context, client *Client, specs []RepoSpec) (*ensuredRepos, error) {
	current, err := client.ListRepos(ctx)
	if err != nil {
		return nil, err
	}
	byFullName := make(map[string]Repo, len(current))
	for _, repo := range current {
		byFullName[repo.FullName] = repo
	}

	result := &ensuredRepos{
		ByFullName: make(map[string]Repo, len(specs)),
		Results:    make([]ApplyRepoResult, 0, len(specs)),
	}

	for _, spec := range specs {
		repo, ok := byFullName[spec.FullName]
		action := "unchanged"
		if !ok {
			created, err := client.CreateRepo(ctx, spec.FullName, spec.WebhookSecret)
			if err != nil {
				return nil, err
			}
			repo = *created
			action = "created"
		} else if strings.TrimSpace(spec.WebhookSecret) != "" && spec.WebhookSecret != repo.WebhookSecret {
			if err := client.UpdateRepoSecret(ctx, repo.ID, spec.WebhookSecret); err != nil {
				return nil, err
			}
			repo.WebhookSecret = spec.WebhookSecret
			action = "updated"
		}

		result.ByFullName[repo.FullName] = repo
		result.Results = append(result.Results, ApplyRepoResult{
			ID:       repo.ID,
			FullName: repo.FullName,
			Action:   action,
		})
	}

	return result, nil
}

func ensureApp(ctx context.Context, client *Client, spec AppSpec) (*App, string, error) {
	apps, err := client.ListApps(ctx)
	if err != nil {
		return nil, "", err
	}
	for _, app := range apps {
		if app.AppKey != spec.AppKey {
			continue
		}
		if app.Name != spec.Name {
			return nil, "", fmt.Errorf("app %q already exists with name %q; rename it manually or update manifest", spec.AppKey, app.Name)
		}
		return &app, "unchanged", nil
	}

	created, err := client.CreateApp(ctx, spec.AppKey, spec.Name)
	if err != nil {
		return nil, "", err
	}
	return created, "created", nil
}

func ensureEnvs(ctx context.Context, client *Client, appID string, envNames []string) ([]ApplyEnvResult, error) {
	current, err := client.ListEnvs(ctx, appID)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]Env, len(current))
	for _, env := range current {
		byName[env.Name] = env
	}

	out := make([]ApplyEnvResult, 0, len(envNames))
	for _, name := range envNames {
		env, ok := byName[name]
		action := "unchanged"
		if !ok {
			created, err := client.CreateEnv(ctx, appID, name)
			if err != nil {
				return nil, err
			}
			env = *created
			action = "created"
		}
		out = append(out, ApplyEnvResult{
			ID:     env.ID,
			Name:   env.Name,
			Action: action,
		})
	}
	return out, nil
}

func ensureServices(ctx context.Context, client *Client, appID string, specs []ServiceSpec, reposByFullName map[string]Repo) ([]ApplyServiceResult, error) {
	current, err := client.ListServices(ctx, appID)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]Service, len(current))
	for _, svc := range current {
		byKey[svc.ServiceKey] = svc
	}

	out := make([]ApplyServiceResult, 0, len(specs))
	for _, spec := range specs {
		svc, ok := byKey[spec.ServiceKey]
		action := "updated"
		if !ok {
			created, err := client.CreateService(ctx, appID, ServiceCreateRequest{
				ServiceKey:    spec.ServiceKey,
				Name:          spec.Name,
				Image:         spec.Image,
				Command:       spec.Command,
				ContainerPort: spec.ContainerPort,
				RunUser:       spec.RunUser,
				Env:           spec.Env,
				ProdHost:      spec.ProdHost,
			})
			if err != nil {
				return nil, err
			}
			svc = *created
			action = "created"
		}

		updated, err := client.UpdateService(ctx, svc.ID, ServiceUpdateRequest{
			Name:               spec.Name,
			ContainerPort:      spec.ContainerPort,
			Env:                spec.Env,
			ProdHost:           spec.ProdHost,
			TraefikEntrypoints: spec.TraefikEntrypoints,
			ComposeTemplate:    spec.ComposeTemplate,
			DeployStrategy:     spec.DeployStrategy,
			Enabled:            spec.EnabledValue(),
		})
		if err != nil {
			return nil, err
		}
		slots, err := ensureSlots(ctx, client, updated.ID, spec.Slots, reposByFullName)
		if err != nil {
			return nil, err
		}
		out = append(out, ApplyServiceResult{
			ID:      updated.ID,
			Key:     updated.ServiceKey,
			Name:    updated.Name,
			Action:  action,
			Slots:   slots,
			Enabled: updated.Enabled,
		})
	}
	return out, nil
}

func ensureSlots(ctx context.Context, client *Client, serviceID string, specs []SlotSpec, reposByFullName map[string]Repo) ([]ApplySlotResult, error) {
	current, err := client.ListSlots(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]Slot, len(current))
	for _, slot := range current {
		byKey[slot.SlotKey] = slot
	}

	out := make([]ApplySlotResult, 0, len(specs))
	for _, spec := range specs {
		repoIDs := make([]string, 0, len(spec.Repos))
		for _, fullName := range spec.Repos {
			repo, ok := reposByFullName[fullName]
			if !ok {
				return nil, fmt.Errorf("slot %q references unknown repo %q", spec.SlotKey, fullName)
			}
			repoIDs = append(repoIDs, repo.ID)
		}
		slices.Sort(repoIDs)
		repoIDs = slices.Compact(repoIDs)

		req := SlotUpsertRequest{
			SlotKey:       spec.SlotKey,
			Name:          spec.Name,
			RepoIDs:       repoIDs,
			MountType:     spec.MountType,
			ContainerPath: spec.ContainerPath,
		}

		slot, ok := byKey[spec.SlotKey]
		action := "updated"
		if !ok {
			created, err := client.CreateSlot(ctx, serviceID, req)
			if err != nil {
				return nil, err
			}
			slot = *created
			action = "created"
		} else {
			updated, err := client.UpdateSlot(ctx, serviceID, slot.ID, req)
			if err != nil {
				return nil, err
			}
			slot = *updated
		}

		out = append(out, ApplySlotResult{
			ID:        slot.ID,
			SlotKey:   slot.SlotKey,
			Action:    action,
			RepoIDs:   slot.RepoIDs,
			MountType: slot.MountType,
		})
	}

	return out, nil
}

func ensureToken(ctx context.Context, client *Client, spec APITokenSpec) (*ApplyTokenResult, error) {
	tokens, err := client.ListTokens(ctx)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		if token.Name != spec.Name {
			continue
		}
		if !spec.RotateIfExists {
			return nil, fmt.Errorf("token %q already exists; set api_token.rotate_if_exists=true to rotate and reveal a new plain token", spec.Name)
		}
		if err := client.RevokeToken(ctx, token.ID); err != nil {
			return nil, err
		}
		created, err := client.CreateToken(ctx, spec.Name)
		if err != nil {
			return nil, err
		}
		return &ApplyTokenResult{
			ID:         created.Token.ID,
			Name:       created.Token.Name,
			Action:     "rotated",
			PlainToken: created.PlainToken,
		}, nil
	}

	created, err := client.CreateToken(ctx, spec.Name)
	if err != nil {
		return nil, err
	}
	return &ApplyTokenResult{
		ID:         created.Token.ID,
		Name:       created.Token.Name,
		Action:     "created",
		PlainToken: created.PlainToken,
	}, nil
}
