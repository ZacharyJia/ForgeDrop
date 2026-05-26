package bootstrap

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

func Export(ctx context.Context, client *Client, appKey string) (*Manifest, error) {
	if client == nil {
		return nil, fmt.Errorf("client is required")
	}
	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return nil, fmt.Errorf("app key is required")
	}
	if err := client.AdminMe(ctx); err != nil {
		return nil, fmt.Errorf("admin token check failed: %w", err)
	}

	settings, err := client.GetSettings(ctx)
	if err != nil {
		return nil, err
	}

	apps, err := client.ListApps(ctx)
	if err != nil {
		return nil, err
	}

	var app *App
	for i := range apps {
		if apps[i].AppKey == appKey {
			app = &apps[i]
			break
		}
	}
	if app == nil {
		return nil, fmt.Errorf("app %q not found", appKey)
	}

	envs, err := client.ListEnvs(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	manifestEnvs := make([]EnvSpec, 0, len(envs))
	for _, env := range envs {
		if env.Kind != "named" {
			continue
		}
		manifestEnvs = append(manifestEnvs, EnvSpec{Name: env.Name})
	}
	slices.SortFunc(manifestEnvs, func(a, b EnvSpec) int {
		return strings.Compare(a.Name, b.Name)
	})

	repos, err := client.ListRepos(ctx)
	if err != nil {
		return nil, err
	}
	reposByID := make(map[string]Repo, len(repos))
	for _, repo := range repos {
		reposByID[repo.ID] = repo
	}

	services, err := client.ListServices(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(services, func(a, b Service) int {
		return strings.Compare(a.ServiceKey, b.ServiceKey)
	})

	usedRepoIDs := make(map[string]struct{})
	serviceSpecs := make([]ServiceSpec, 0, len(services))
	for _, svc := range services {
		slots, err := client.ListSlots(ctx, svc.ID)
		if err != nil {
			return nil, err
		}
		slices.SortFunc(slots, func(a, b Slot) int {
			return strings.Compare(a.SlotKey, b.SlotKey)
		})

		slotSpecs := make([]SlotSpec, 0, len(slots))
		for _, slot := range slots {
			repoNames := make([]string, 0, len(slot.RepoIDs))
			for _, repoID := range slot.RepoIDs {
				repo, ok := reposByID[repoID]
				if !ok {
					return nil, fmt.Errorf("slot %q references unknown repo id %q", slot.SlotKey, repoID)
				}
				repoNames = append(repoNames, repo.FullName)
				usedRepoIDs[repoID] = struct{}{}
			}
			slices.Sort(repoNames)
			repoNames = slices.Compact(repoNames)

			slotSpecs = append(slotSpecs, SlotSpec{
				SlotKey:       slot.SlotKey,
				Name:          slot.Name,
				Repos:         repoNames,
				MountType:     slot.MountType,
				ContainerPath: slot.ContainerPath,
			})
		}

		enabled := svc.Enabled
		serviceSpecs = append(serviceSpecs, ServiceSpec{
			ServiceKey:         svc.ServiceKey,
			Name:               svc.Name,
			Image:              svc.Image,
			Command:            svc.Command,
			ContainerPort:      svc.ContainerPort,
			RunUser:            svc.RunUser,
			Env:                cloneStringMap(svc.Env),
			ProdHost:           svc.ProdHost,
			TraefikEntrypoints: svc.TraefikEntrypoints,
			ComposeTemplate:    svc.ComposeTemplate,
			DeployStrategy:     svc.DeployStrategy,
			Enabled:            &enabled,
			Slots:              slotSpecs,
		})
	}

	repoSpecs := make([]RepoSpec, 0, len(usedRepoIDs))
	for repoID := range usedRepoIDs {
		repo, ok := reposByID[repoID]
		if !ok {
			return nil, fmt.Errorf("repo id %q not found", repoID)
		}
		repoSpecs = append(repoSpecs, RepoSpec{FullName: repo.FullName})
	}
	slices.SortFunc(repoSpecs, func(a, b RepoSpec) int {
		return strings.Compare(a.FullName, b.FullName)
	})

	manifest := &Manifest{
		Settings: settings,
		Repos:    repoSpecs,
		App: AppSpec{
			AppKey:   app.AppKey,
			Name:     app.Name,
			Envs:     manifestEnvs,
			Services: serviceSpecs,
		},
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("exported manifest invalid: %w", err)
	}
	return manifest, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
