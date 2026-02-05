// API client for forge-drop

export interface User {
  id: string;
  username: string;
}

export interface App {
  id: string;
  app_key: string;
  name: string;
  created_at: string;
}

export interface Service {
  id: string;
  app_id: string;
  service_key: string;
  name: string;
  image: string;
  command: string;
  container_port: number;
  run_user: string;
  env: Record<string, string>;
  prod_host: string;
  traefik_entrypoints: string;
  compose_template: string;
  use_compose: boolean;
  revision: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface Env {
  id: string;
  app_id: string;
  kind: string;
  name: string;
  created_at: string;
  current_snapshot_id?: string | null;
}

export interface Slot {
  id: string;
  service_id: string;
  slot_key: string;
  name: string;
  repo_id: string;
  container_path: string;
  created_at: string;
  updated_at: string;
}

export interface Repo {
  id: string;
  full_name: string;
  slug: string;
  webhook_secret: string;
  created_at: string;
}

export interface APIToken {
  id: string;
  name: string;
  prefix: string;
  created_at: string;
  revoked_at?: string;
}

export interface Settings {
  base_domain: string;
  preview_host_template: string;
  docker_network: string;
  artifact_upload_url: string;
  forgejo_webhook_url: string;
}

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });
  const txt = await res.text();
  const data = txt ? JSON.parse(txt) : null;
  if (!res.ok) throw new Error(data?.error || `${res.status} ${res.statusText}`);
  return data as T;
}

export const api = {
  // Setup
  async getSetupStatus(): Promise<{ allowed: boolean; user_count: number }> {
    return apiFetch('/api/v1/setup/status');
  },

  async setup(username: string, password: string): Promise<{ user_id: string }> {
    return apiFetch('/api/v1/setup', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    });
  },

  // Auth
  async getMe(): Promise<User> {
    return apiFetch('/api/v1/admin/me');
  },

  async login(username: string, password: string): Promise<void> {
    await apiFetch('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    });
  },

  async logout(): Promise<void> {
    await apiFetch('/api/v1/auth/logout', { method: 'POST' });
  },

  // Settings
  async getSettings(): Promise<Settings> {
    return apiFetch('/api/v1/admin/settings');
  },

  async updateSettings(settings: Partial<Settings>): Promise<void> {
    await apiFetch('/api/v1/admin/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    });
  },

  // Apps
  async listApps(): Promise<App[]> {
    return apiFetch('/api/v1/admin/apps');
  },

  async createApp(appKey: string, name: string): Promise<App> {
    return apiFetch('/api/v1/admin/apps', {
      method: 'POST',
      body: JSON.stringify({ app_key: appKey, name }),
    });
  },

  async getApp(appId: string): Promise<{ app: App; services: Service[]; envs: Env[] }> {
    return apiFetch(`/api/v1/admin/apps/${appId}`);
  },

  async createService(appId: string, payload: {
    service_key: string;
    name: string;
    image?: string;
    command?: string;
    container_port?: number;
    run_user?: string;
    env?: Record<string, string>;
    prod_host?: string;
  }): Promise<Service> {
    return apiFetch(`/api/v1/admin/apps/${appId}/services`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },

  async createEnv(appId: string, name: string): Promise<Env> {
    return apiFetch(`/api/v1/admin/apps/${appId}/envs`, {
      method: 'POST',
      body: JSON.stringify({ name }),
    });
  },

  async deleteApp(appId: string): Promise<void> {
    await apiFetch(`/api/v1/admin/apps/${appId}`, { method: 'DELETE' });
  },

  // Services
  async getService(serviceId: string): Promise<{ service: Service; slots: Slot[] }> {
    return apiFetch(`/api/v1/admin/services/${serviceId}`);
  },

  async getServiceStatus(serviceId: string, envId: string): Promise<any> {
    const q = new URLSearchParams({ env_id: envId });
    return apiFetch(`/api/v1/admin/services/${serviceId}/status?${q.toString()}`);
  },

  async getServiceLogs(serviceId: string, envId: string, tail = 200): Promise<{ logs: string }> {
    const q = new URLSearchParams({ env_id: envId, tail: String(tail) });
    return apiFetch(`/api/v1/admin/services/${serviceId}/logs?${q.toString()}`);
  },

  async deployService(serviceId: string, envId: string): Promise<any> {
    return apiFetch(`/api/v1/admin/services/${serviceId}/deploy`, {
      method: 'POST',
      body: JSON.stringify({ env_id: envId }),
    });
  },

  async createSlot(serviceId: string, payload: {
    slot_key: string;
    name: string;
    repo_id: string;
    container_path: string;
  }): Promise<Slot> {
    return apiFetch(`/api/v1/admin/services/${serviceId}/slots`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },

  async deleteSlot(serviceId: string, slotId: string): Promise<void> {
    await apiFetch(`/api/v1/admin/services/${serviceId}/slots/${slotId}`, { method: 'DELETE' });
  },

  async uploadArtifactsBatch(serviceId: string, formData: FormData): Promise<any> {
    // Must send cookies; Content-Type is set by the browser for multipart
    const res = await fetch(`/api/v1/admin/services/${serviceId}/artifacts/upload-batch`, {
      method: 'POST',
      credentials: 'same-origin',
      body: formData,
    });
    const txt = await res.text();
    const data = txt ? JSON.parse(txt) : null;
    if (!res.ok) throw new Error(data?.error || `${res.status} ${res.statusText}`);
    return data;
  },

  async updateService(serviceId: string, data: Partial<Service>): Promise<Service> {
    return apiFetch(`/api/v1/admin/services/${serviceId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },

  async deleteService(serviceId: string): Promise<void> {
    await apiFetch(`/api/v1/admin/services/${serviceId}`, { method: 'DELETE' });
  },

  async getComposeTemplateExample(serviceId: string): Promise<{ example: string; description: string }> {
    return apiFetch(`/api/v1/admin/services/${serviceId}/compose-template-example`);
  },

  // Repos
  async listRepos(): Promise<Repo[]> {
    return apiFetch('/api/v1/admin/repos');
  },

  async createRepo(fullName: string, webhookSecret: string): Promise<Repo> {
    return apiFetch('/api/v1/admin/repos', {
      method: 'POST',
      body: JSON.stringify({ full_name: fullName, webhook_secret: webhookSecret }),
    });
  },

  async deleteRepo(repoId: string): Promise<void> {
    await apiFetch(`/api/v1/admin/repos/${repoId}`, { method: 'DELETE' });
  },

  // Tokens
  async listTokens(): Promise<APIToken[]> {
    return apiFetch('/api/v1/admin/tokens');
  },

  async createToken(name: string): Promise<{ token: APIToken; plain_token: string }> {
    return apiFetch('/api/v1/admin/tokens', {
      method: 'POST',
      body: JSON.stringify({ name }),
    });
  },

  async revokeToken(tokenId: string): Promise<void> {
    await apiFetch(`/api/v1/admin/tokens/${tokenId}`, { method: 'DELETE' });
  },
};
