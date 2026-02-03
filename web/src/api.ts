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

  async getApp(appId: string): Promise<{ app: App; services: Service[]; envs: any[] }> {
    return apiFetch(`/api/v1/admin/apps/${appId}`);
  },

  async deleteApp(appId: string): Promise<void> {
    await apiFetch(`/api/v1/admin/apps/${appId}`, { method: 'DELETE' });
  },

  // Services
  async getService(serviceId: string): Promise<{ service: Service; slots: any[] }> {
    return apiFetch(`/api/v1/admin/services/${serviceId}`);
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

  async getComposeTemplateExample(): Promise<{ example: string; description: string }> {
    return apiFetch('/api/v1/admin/services/dummy/compose-template-example');
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
