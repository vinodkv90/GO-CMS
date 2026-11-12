import { api, ApiError, getStoredToken } from './client';
import type {
  User,
  ContentType,
  ContentEntry,
  ContentVersion,
  MediaAsset,
  MediaFolder,
  MediaFolderTreeNode,
  PluginEntity,
  ThemeManifest,
  PublicGeneralSettings,
  GeneralSettings,
  SettingsOverview,
  SystemInfo,
  UserPreferences,
  Webhook,
  WebhookDelivery,
  WebhookEventDescription,
  CreateWebhookPayload,
  UpdateWebhookPayload,
  Locale,
  CreateLocalePayload,
  UpdateLocalePayload,
  AdminInvitation,
  InvitationPreview,
  Permission,
  Role,
  TokenKind,
  ApiToken,
  CreateTokenPayload,
  RestAPISettings,
  CORSSettings,
  RateLimitSettings,
  MediaSettings,
  StorageSettings,
  StorageTestResult,
  EmailSettings,
  EmailTestResult,
  EmailTemplate,
  UpdateEmailTemplateInput,
  EmailTemplatePreview,
  SecuritySettings,
  PasswordPolicySettings,
  SSOSettings,
  LoginResponse,
  MFAEnrollResult,
  MFAConfirmResult,
  AppRole,
  AppUser,
  OAuthProvider,
  PublicProviderInfo,
  AppAuthSettings,
  OIDCDiscoveryResult,
  AuditLogEntry,
  AuditLogFilter,
} from '../types';

export const authApi = {
  login: (credentials: { username: string; password: string; identifier?: string }) =>
    api.post<LoginResponse>('/auth/login', {
      identifier: credentials.username,
      username: credentials.username,
      password: credentials.password,
    }),

  me: () => api.get<User | { data?: User; user?: User }>('/auth/me'),

  updateMe: (data: { email: string; first_name: string; last_name: string }) =>
    api.patch<User>('/auth/me', data),

  changePassword: (data: { current_password: string; new_password: string }) =>
    api.post<{ message: string }>('/auth/change-password', data),

  previewInvitation: (token: string) => {
    const q = new URLSearchParams({ token });
    return api.get<InvitationPreview>(`/auth/invitations/preview?${q.toString()}`);
  },

  acceptInvitation: (data: {
    token: string;
    username?: string;
    password: string;
    first_name?: string;
    last_name?: string;
  }) => api.post<{ token: string; user: User }>('/auth/invitations/accept', data),

  verifyMfa: (mfa_token: string, code: string) =>
    api.post<LoginResponse>('/auth/mfa/verify', { mfa_token, code }),

  enrollMfa: (mfa_token?: string) =>
    api.post<{ data: MFAEnrollResult }>('/auth/mfa/enroll', mfa_token ? { mfa_token } : {}),

  confirmMfa: (code: string, mfa_token?: string) =>
    api.post<{ data: MFAConfirmResult }>('/auth/mfa/confirm', mfa_token ? { code, mfa_token } : { code }),

  disableMfa: (password: string, code: string) =>
    api.post<{ data: User }>('/auth/mfa/disable', { password, code }),

  regenerateRecoveryCodes: (code: string) =>
    api.post<{ data: { recovery_codes: string[] } }>('/auth/mfa/regenerate-recovery-codes', { code }),

  resetExpiredPassword: (data: {
    identifier: string;
    current_password: string;
    new_password: string;
  }) => api.post<LoginResponse>('/auth/reset-expired-password', data),

  listSsoProviders: () =>
    api.get<{ data: { enabled: boolean; providers: PublicProviderInfo[] } }>('/auth/sso/providers'),

  logout: () => api.post('/auth/logout'),
};

export const contentTypesApi = {
  list: () => api.get<{ data: ContentType[] }>('/content-types'),

  get: (uid: string) => api.get<ContentType>(`/content-types/${uid}`),

  update: (uid: string, data: { name?: string; settings?: Record<string, any> }) =>
    api.put<ContentType>(`/content-types/${uid}`, data),

  delete: (uid: string) => api.delete(`/content-types/${uid}`),

  create: (typeData: {
    name: string;
    uid: string;
    kind: 'collection' | 'single' | 'component';
    settings?: Record<string, unknown>;
    fields?: Array<{
      name: string;
      field_key: string;
      field_type: string;
      configuration?: Record<string, any>;
    }>;
  }) => api.post<ContentType>('/content-types', typeData),

  addField: (uid: string, field: any) =>
    api.post<{ data: ContentType }>(`/content-types/${uid}/fields`, field),

  updateField: (uid: string, fieldKey: string, field: any) =>
    api.put<{ message: string }>(`/content-types/${uid}/fields/${fieldKey}`, field),

  deleteField: (uid: string, fieldKey: string) =>
    api.delete<{ message: string }>(`/content-types/${uid}/fields/${fieldKey}`),
};

export const entriesApi = {
  list: async (typeUID: string, params?: { limit?: number; offset?: number; preview?: boolean; search?: string; status?: string; sort?: string; order?: string }) => {
    const q = new URLSearchParams();
    if (params?.limit) q.set('limit', String(params.limit));
    if (params?.offset) q.set('offset', String(params.offset));
    if (params?.preview) q.set('preview', 'true');
    if (params?.search) q.set('search', params.search);
    if (params?.status) q.set('status', params.status);
    if (params?.sort) q.set('sort', params.sort);
    if (params?.order) q.set('order', params.order);
    const response = await api.get<{ data: ContentEntry[]; meta?: { total?: number }; total?: number } | ContentEntry>(
      `/content/${typeUID}?${q.toString()}`
    );
    // Collection endpoints return a list envelope, while single content types
    // return their one entry directly. Normalize both shapes for callers.
    if (Array.isArray((response as { data?: unknown }).data)) {
      const listResponse = response as { data: ContentEntry[]; meta?: { total?: number }; total?: number };
      return {
        data: listResponse.data,
        meta: { total: listResponse.meta?.total ?? listResponse.total ?? listResponse.data.length },
      };
    }
    return { data: response ? [response as ContentEntry] : [], meta: { total: response ? 1 : 0 } };
  },

  get: (typeUID: string, id: string, preview = true) =>
    api.get<ContentEntry>(`/content/${typeUID}/${id}${preview ? '?preview=true' : ''}`),

  create: (typeUID: string, data: Record<string, any>) =>
    api.post<ContentEntry>(`/content/${typeUID}`, { data }),

  update: (typeUID: string, id: string, data: Record<string, any>, expectedVersion: number) =>
    api.put<ContentEntry>(`/content/${typeUID}/${id}`, {
      data,
      expected_draft_version: expectedVersion,
    }),

  publish: (typeUID: string, id: string) =>
    api.post<ContentEntry>(`/content/${typeUID}/${id}/publish`),

  unpublish: (typeUID: string, id: string) =>
    api.post<ContentEntry>(`/content/${typeUID}/${id}/unpublish`),

  delete: (typeUID: string, id: string) => api.delete(`/content/${typeUID}/${id}`),

  versions: (typeUID: string, id: string) =>
    api.get<{ data: ContentVersion[] }>(`/content/${typeUID}/${id}/versions`),

  restore: (typeUID: string, id: string, versionNumber: number) =>
    api.post<ContentEntry>(`/content/${typeUID}/${id}/restore/${versionNumber}`),
};

export const mediaApi = {
  get: (id: string) => api.get<{ data: MediaAsset }>(`/media/${id}`),

  list: (params?: { search?: string; mime_type?: string; limit?: number; offset?: number; folder_id?: string | null }) => {
    const q = new URLSearchParams();
    if (params?.search) q.set('search', params.search);
    if (params?.mime_type) q.set('mime_type', params.mime_type);
    if (params?.limit) q.set('limit', String(params.limit));
    if (params?.offset) q.set('offset', String(params.offset));
    if (params?.folder_id !== undefined && params?.folder_id !== null) {
      q.set('folder_id', params.folder_id);
    }
    return api.get<{ data: MediaAsset[]; meta: { total: number } }>(`/media?${q.toString()}`);
  },

  upload: (file: File, altText = '', caption = '', folderId?: string | null) => {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('alt_text', altText);
    formData.append('caption', caption);
    if (folderId) {
      formData.append('folder_id', folderId);
    }
    return api.upload<{ data: MediaAsset }>('/media', formData);
  },

  uploadHls: (file: File, title = '', folderId?: string | null) => {
    const formData = new FormData();
    formData.append('file', file);
    if (title) {
      formData.append('title', title);
    }
    if (folderId) {
      formData.append('folder_id', folderId);
    }
    return api.upload<{ data: MediaAsset }>('/media/hls', formData);
  },

  move: (id: string, targetFolderId?: string | null) =>
    api.post<{ data: MediaAsset }>(`/media/${id}/move`, { target_folder_id: targetFolderId || null }),

  delete: (id: string) => api.delete(`/media/${id}`),

  getSignedURL: (id: string, ttl = 3600) =>
    api.get<{ signed_url: string; expires_in: number }>(`/media/${id}/signed-url?ttl=${ttl}`),
};

export const mediaFoldersApi = {
  tree: () => api.get<{ data: MediaFolderTreeNode[] }>('/media/tree'),

  list: (parentId?: string) => {
    const q = parentId !== undefined ? `?parent_id=${encodeURIComponent(parentId)}` : '';
    return api.get<{ data: MediaFolder[] }>(`/media/folders${q}`);
  },

  get: (id: string) => api.get<{ data: MediaFolder }>(`/media/folders/${id}`),

  create: (data: { name: string; parent_id?: string | null }) =>
    api.post<{ data: MediaFolder }>('/media/folders', data),

  rename: (id: string, name: string) =>
    api.put<{ data: MediaFolder }>(`/media/folders/${id}`, { name }),

  delete: (id: string, recursive: boolean = false) =>
    api.delete(`/media/folders/${id}?recursive=${recursive}`),

  contents: (id: string) =>
    api.get<{ data: { folder: MediaFolder; subfolders: MediaFolder[]; assets: MediaAsset[] } }>(`/media/folders/${id}/contents`),

  moveFolder: (id: string, targetParentId?: string | null) =>
    api.post<{ data: MediaFolder }>(`/media/folders/${id}/move`, { target_parent_id: targetParentId || null }),

  moveAsset: (assetId: string, targetFolderId?: string | null) =>
    api.post<{ data: MediaAsset }>(`/media/${assetId}/move`, { target_folder_id: targetFolderId || null }),
};

export const pluginsApi = {
  list: () => api.get<{ data: PluginEntity[] }>('/plugins'),

  get: (id: string) => api.get<{ data: PluginEntity }>(`/plugins/${id}`),

  enable: (id: string) => api.post<{ message: string; data: PluginEntity }>(`/plugins/${id}/enable`),

  disable: (id: string) => api.post<{ message: string; data: PluginEntity }>(`/plugins/${id}/disable`),

  updateSettings: (id: string, settings: Record<string, any>) =>
    api.patch<{ message: string; data: PluginEntity }>(`/plugins/${id}/settings`, settings),
};

export const themesApi = {
  list: () => api.get<{ data: ThemeManifest[] }>('/themes'),
};

export const usersApi = {
  list: (params?: { search?: string; status?: string; role_id?: string; limit?: number; offset?: number }) => {
    const q = new URLSearchParams();
    if (params?.search) q.set('search', params.search);
    if (params?.status) q.set('status', params.status);
    if (params?.role_id) q.set('role_id', params.role_id);
    if (params?.limit !== undefined) q.set('limit', String(params.limit));
    if (params?.offset !== undefined) q.set('offset', String(params.offset));
    const qs = q.toString();
    return api.get<{ data: User[]; total?: number }>(qs ? `/users?${qs}` : '/users');
  },

  get: (id: string) => api.get<User>(`/users/${id}`),

  create: (data: {
    username: string;
    email: string;
    password: string;
    first_name?: string;
    last_name?: string;
    status?: User['status'];
    role_ids: string[];
  }) => api.post<User>('/users', data),

  update: (
    id: string,
    data: {
      email?: string;
      first_name?: string;
      last_name?: string;
      status?: User['status'];
      role_ids?: string[];
    },
  ) => api.patch<User>(`/users/${id}`, data),

  activate: (id: string) => api.post<User>(`/users/${id}/activate`),

  deactivate: (id: string) => api.post<User>(`/users/${id}/deactivate`),

  unlock: (id: string) => api.post<User>(`/users/${id}/unlock`),

  remove: (id: string) => api.delete<{ message: string }>(`/users/${id}`),

  listInvitations: () => api.get<{ data: AdminInvitation[] }>('/users/invitations'),

  createInvitation: (data: {
    email: string;
    username?: string;
    first_name?: string;
    last_name?: string;
    role_ids: string[];
    expires_in_hours?: number;
  }) => api.post<{ invitation: AdminInvitation; token: string }>('/users/invitations', data),

  resendInvitation: (id: string, expiresInHours?: number) =>
    api.post<{ invitation: AdminInvitation; token: string }>(`/users/invitations/${id}/resend`, {
      expires_in_hours: expiresInHours,
    }),

  revokeInvitation: (id: string) => api.post<AdminInvitation>(`/users/invitations/${id}/revoke`),

  deleteInvitation: (id: string) => api.delete<{ message: string }>(`/users/invitations/${id}`),
};

export const rolesApi = {
  list: () => api.get<{ data: Role[] }>('/roles'),

  get: (id: string) => api.get<Role>(`/roles/${id}`),

  create: (data: { name: string; slug?: string; description?: string; permission_ids: string[] }) =>
    api.post<Role>('/roles', data),

  update: (id: string, data: { name?: string; slug?: string; description?: string; permission_ids?: string[] }) =>
    api.put<Role>(`/roles/${id}`, data),

  duplicate: (id: string, data?: { name?: string; slug?: string; description?: string }) =>
    api.post<Role>(`/roles/${id}/duplicate`, data || {}),

  remove: (id: string) => api.delete<{ message: string }>(`/roles/${id}`),

  listPermissions: () => api.get<{ data: Permission[] }>('/permissions'),
};

export const settingsApi = {
  getRestApi: () => api.get<{ data: RestAPISettings }>('/settings/rest-api'),
  updateRestApi: (data: RestAPISettings) => api.put<{ data: RestAPISettings }>('/settings/rest-api', data),
  getCors: () => api.get<{ data: CORSSettings }>('/settings/cors'),
  updateCors: (data: CORSSettings) => api.put<{ data: CORSSettings }>('/settings/cors', data),
  getRateLimit: () => api.get<{ data: RateLimitSettings }>('/settings/rate-limit'),
  updateRateLimit: (data: RateLimitSettings) => api.put<{ data: RateLimitSettings }>('/settings/rate-limit', data),
  getMedia: () => api.get<{ data: MediaSettings }>('/settings/media'),
  updateMedia: (data: MediaSettings) => api.put<{ data: MediaSettings }>('/settings/media', data),
  getStorage: () => api.get<{ data: StorageSettings }>('/settings/storage'),
  updateStorage: (data: StorageSettings) => api.put<{ data: StorageSettings }>('/settings/storage', data),
  testStorage: (override?: StorageSettings) =>
    api.post<{ data: StorageTestResult }>('/settings/storage/test', override),
  getSecurity: () => api.get<{ data: SecuritySettings }>('/settings/security'),
  updateSecurity: (data: SecuritySettings) => api.put<{ data: SecuritySettings }>('/settings/security', data),
  getPasswordPolicy: () => api.get<{ data: PasswordPolicySettings }>('/settings/password-policy'),
  updatePasswordPolicy: (data: PasswordPolicySettings) =>
    api.put<{ data: PasswordPolicySettings }>('/settings/password-policy', data),
  getSso: () => api.get<{ data: SSOSettings }>('/settings/sso'),
  updateSso: (data: SSOSettings) => api.put<{ data: SSOSettings }>('/settings/sso', data),
  getPublic: () => api.get<{ data: PublicGeneralSettings }>('/settings/public'),

  getOverview: () => api.get<{ data: SettingsOverview }>('/settings/overview'),

  getGeneral: () => api.get<{ data: GeneralSettings }>('/settings/general'),

  updateGeneral: (data: GeneralSettings) =>
    api.put<{ data: GeneralSettings }>('/settings/general', data),

  getSystemInfo: () => api.get<{ data: SystemInfo }>('/settings/system'),
};

export const preferencesApi = {
  get: () => api.get<{ data: UserPreferences }>('/preferences'),

  update: (data: UserPreferences) => api.put<{ data: UserPreferences }>('/preferences', data),
};

export const tokensApi = {
  list: (kind?: TokenKind) => {
    const q = kind ? `?${new URLSearchParams({ kind }).toString()}` : '';
    return api.get<{ data: ApiToken[] }>(`/api-tokens${q}`);
  },
  create: (data: CreateTokenPayload) =>
    api.post<{ token: string; metadata: ApiToken }>('/api-tokens', data),
  rotate: (id: string) =>
    api.post<{ token: string; metadata: ApiToken }>(`/api-tokens/${id}/rotate`),
  revoke: (id: string) => api.post<{ message: string }>(`/api-tokens/${id}/revoke`),
  remove: (id: string) => api.delete<{ message: string }>(`/api-tokens/${id}`),

  listTransfer: () => api.get<{ data: ApiToken[] }>('/transfer-tokens'),
  createTransfer: (data: Omit<CreateTokenPayload, 'kind'>) =>
    api.post<{ token: string; metadata: ApiToken }>('/transfer-tokens', {
      ...data,
      kind: 'transfer',
    }),
};


export const emailApi = {
  getSettings: () => api.get<{ data: EmailSettings }>('/settings/email'),
  updateSettings: (data: EmailSettings) => api.put<{ data: EmailSettings }>('/settings/email', data),
  testConnection: (override?: EmailSettings) =>
    api.post<{ data: EmailTestResult }>('/settings/email/test-connection', override),
  testSend: (to: string, override?: EmailSettings) =>
    api.post<{ data: EmailTestResult }>('/settings/email/test-send', { to, override }),
};

export const emailTemplatesApi = {
  list: () => api.get<{ data: EmailTemplate[] }>('/settings/email-templates'),
  get: (key: string) => api.get<{ data: EmailTemplate }>(`/settings/email-templates/${encodeURIComponent(key)}`),
  update: (key: string, data: UpdateEmailTemplateInput) =>
    api.put<{ data: EmailTemplate }>(`/settings/email-templates/${encodeURIComponent(key)}`, data),
  preview: (key: string, override?: Partial<UpdateEmailTemplateInput>, variables?: Record<string, string>) =>
    api.post<{ data: EmailTemplatePreview }>(`/settings/email-templates/${encodeURIComponent(key)}/preview`, {
      override,
      variables,
    }),
};


export const usersPermApi = {
  listRoles: () => api.get<{ data: AppRole[] }>('/admin/users-permissions/roles'),
  getRole: (id: string) => api.get<{ data: AppRole }>(`/admin/users-permissions/roles/${id}`),
  createRole: (data: {
    name: string;
    slug?: string;
    description?: string;
    is_default?: boolean;
    permissions: Record<string, Record<string, boolean>>;
  }) => api.post<{ data: AppRole }>('/admin/users-permissions/roles', data),
  updateRole: (
    id: string,
    data: {
      name: string;
      description?: string;
      is_default?: boolean;
      permissions: Record<string, Record<string, boolean>>;
    },
  ) => api.put<{ data: AppRole }>(`/admin/users-permissions/roles/${id}`, data),
  deleteRole: (id: string) => api.delete<{ message: string }>(`/admin/users-permissions/roles/${id}`),

  listUsers: (params?: { search?: string; role_id?: string; limit?: number; offset?: number }) => {
    const q = new URLSearchParams();
    if (params?.search) q.set('search', params.search);
    if (params?.role_id) q.set('role_id', params.role_id);
    if (params?.limit !== undefined) q.set('limit', String(params.limit));
    if (params?.offset !== undefined) q.set('offset', String(params.offset));
    const qs = q.toString();
    return api.get<{ data: AppUser[]; meta: { total: number; limit: number; offset: number } }>(
      qs ? `/admin/users-permissions/users?${qs}` : '/admin/users-permissions/users',
    );
  },
  createUser: (data: {
    username: string;
    email: string;
    password: string;
    role_id?: string;
    confirmed?: boolean;
    blocked?: boolean;
  }) => api.post<{ data: AppUser }>('/admin/users-permissions/users', data),
  updateUser: (
    id: string,
    data: {
      username?: string;
      email?: string;
      password?: string;
      role_id?: string;
      confirmed?: boolean;
      blocked?: boolean;
    },
  ) => api.put<{ data: AppUser }>(`/admin/users-permissions/users/${id}`, data),
  deleteUser: (id: string) => api.delete<{ message: string }>(`/admin/users-permissions/users/${id}`),

  listProviders: (scope?: 'app' | 'admin') => {
    const qs = scope ? `?${new URLSearchParams({ scope }).toString()}` : '';
    return api.get<{ data: OAuthProvider[] }>(`/admin/users-permissions/providers${qs}`);
  },
  upsertProvider: (
    name: string,
    data: Partial<OAuthProvider> & { client_secret?: string },
  ) => api.put<{ data: OAuthProvider }>(`/admin/users-permissions/providers/${encodeURIComponent(name)}`, data),
  deleteProvider: (name: string) =>
    api.delete<{ message: string }>(`/admin/users-permissions/providers/${encodeURIComponent(name)}`),
  discoverOidc: (issuer_url: string) =>
    api.post<{ data: OIDCDiscoveryResult }>('/admin/users-permissions/providers/discover', { issuer_url }),

  getSettings: () => api.get<{ data: AppAuthSettings }>('/admin/users-permissions/settings'),
  updateSettings: (data: AppAuthSettings) =>
    api.put<{ data: AppAuthSettings }>('/admin/users-permissions/settings', data),
};

export const webhooksApi = {
  list: () => api.get<{ data: Webhook[] }>('/webhooks'),
  listEvents: () => api.get<{ data: WebhookEventDescription[] }>('/webhooks/events'),
  get: (id: string) => api.get<{ data: Webhook }>(`/webhooks/${encodeURIComponent(id)}`),
  create: (data: CreateWebhookPayload) => api.post<{ data: Webhook }>('/webhooks', data),
  update: (id: string, data: UpdateWebhookPayload) =>
    api.put<{ data: Webhook }>(`/webhooks/${encodeURIComponent(id)}`, data),
  remove: (id: string) => api.delete<{ message: string }>(`/webhooks/${encodeURIComponent(id)}`),
  triggerTest: (id: string) => api.post<{ data: WebhookDelivery }>(`/webhooks/${encodeURIComponent(id)}/trigger`),
  listDeliveries: (id: string, limit = 20, offset = 0) =>
    api.get<{ data: WebhookDelivery[]; meta: { total: number; limit: number; offset: number } }>(
      `/webhooks/${encodeURIComponent(id)}/deliveries?limit=${limit}&offset=${offset}`,
    ),
};

export const localesApi = {
  list: () => api.get<{ data: Locale[] }>('/locales'),
  get: (code: string) => api.get<{ data: Locale }>(`/locales/${encodeURIComponent(code)}`),
  create: (data: CreateLocalePayload) => api.post<{ data: Locale }>('/locales', data),
  update: (code: string, data: UpdateLocalePayload) =>
    api.put<{ data: Locale }>(`/locales/${encodeURIComponent(code)}`, data),
  setDefault: (code: string) => api.put<{ message: string }>(`/locales/${encodeURIComponent(code)}/default`),
  remove: (code: string) => api.delete<{ message: string }>(`/locales/${encodeURIComponent(code)}`),
};

function buildAuditQuery(filter?: AuditLogFilter): string {
  if (!filter) return '';
  const params = new URLSearchParams();
  if (filter.actor_id?.trim()) params.set('actor_id', filter.actor_id.trim());
  if (filter.action?.trim()) params.set('action', filter.action.trim());
  if (filter.resource?.trim()) params.set('resource', filter.resource.trim());
  if (filter.result?.trim()) params.set('result', filter.result.trim());
  if (filter.from?.trim()) params.set('from', filter.from.trim());
  if (filter.to?.trim()) params.set('to', filter.to.trim());
  if (typeof filter.limit === 'number') params.set('limit', String(filter.limit));
  if (typeof filter.offset === 'number') params.set('offset', String(filter.offset));
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

export const auditApi = {
  list: (filter?: AuditLogFilter) =>
    api.get<{ data: AuditLogEntry[]; meta: { total: number; limit: number; offset: number } }>(
      `/audit-logs${buildAuditQuery(filter)}`,
    ),
  exportCsv: async (filter?: Omit<AuditLogFilter, 'limit' | 'offset'>): Promise<Blob> => {
    const token = getStoredToken();
    const headers: Record<string, string> = {};
    if (token) headers.Authorization = `Bearer ${token}`;
    const res = await fetch(`/api/v1/audit-logs/export${buildAuditQuery(filter)}`, {
      method: 'GET',
      headers,
    });
    if (!res.ok) {
      let msg = `Export failed with status ${res.status}`;
      try {
        const body = await res.json();
        msg = body.error || body.message || msg;
      } catch {
        // Ignore non-JSON error body
      }
      throw new ApiError(res.status, msg);
    }
    return await res.blob();
  },
};
