import { api } from './client';
import type {
  User,
  ContentType,
  ContentEntry,
  ContentVersion,
  MediaAsset,
  PluginEntity,
  ThemeManifest,
} from '../types';

export const authApi = {
  login: (credentials: { username: string; password: string; identifier?: string }) =>
    api.post<{ token: string; user: User }>('/auth/login', {
      identifier: credentials.username,
      username: credentials.username,
      password: credentials.password,
    }),

  me: () => api.get<{ data: User }>('/auth/me'),

  logout: () => api.post('/auth/logout'),
};

export const contentTypesApi = {
  list: () => api.get<{ data: ContentType[] }>('/content-types'),

  get: (uid: string) => api.get<{ data: ContentType }>(`/content-types/${uid}`),

  create: (typeData: {
    name: string;
    uid: string;
    kind: 'collection' | 'single';
    fields?: Array<{
      name: string;
      field_key: string;
      field_type: string;
      configuration?: Record<string, any>;
    }>;
  }) => api.post<{ data: ContentType }>('/content-types', typeData),

  addField: (uid: string, field: any) =>
    api.post<{ data: ContentType }>(`/content-types/${uid}/fields`, field),
};

export const entriesApi = {
  list: (typeUID: string, params?: { limit?: number; offset?: number; preview?: boolean }) => {
    const q = new URLSearchParams();
    if (params?.limit) q.set('limit', String(params.limit));
    if (params?.offset) q.set('offset', String(params.offset));
    if (params?.preview) q.set('preview', 'true');
    return api.get<{ data: ContentEntry[]; meta: { total: number } }>(
      `/content/${typeUID}?${q.toString()}`
    );
  },

  get: (typeUID: string, id: string, preview = true) =>
    api.get<{ data: ContentEntry }>(`/content/${typeUID}/${id}${preview ? '?preview=true' : ''}`),

  create: (typeUID: string, data: Record<string, any>) =>
    api.post<{ data: ContentEntry }>(`/content/${typeUID}`, { data }),

  update: (typeUID: string, id: string, data: Record<string, any>, expectedVersion: number) =>
    api.patch<{ data: ContentEntry }>(`/content/${typeUID}/${id}`, {
      data,
      expected_version: expectedVersion,
    }),

  publish: (typeUID: string, id: string) =>
    api.post<{ data: ContentEntry }>(`/content/${typeUID}/${id}/publish`),

  unpublish: (typeUID: string, id: string) =>
    api.post<{ data: ContentEntry }>(`/content/${typeUID}/${id}/unpublish`),

  versions: (typeUID: string, id: string) =>
    api.get<{ data: ContentVersion[] }>(`/content/${typeUID}/${id}/versions`),

  restore: (typeUID: string, id: string, versionNumber: number) =>
    api.post<{ data: ContentEntry }>(`/content/${typeUID}/${id}/restore/${versionNumber}`),
};

export const mediaApi = {
  list: (params?: { search?: string; mime_type?: string; limit?: number; offset?: number }) => {
    const q = new URLSearchParams();
    if (params?.search) q.set('search', params.search);
    if (params?.mime_type) q.set('mime_type', params.mime_type);
    if (params?.limit) q.set('limit', String(params.limit));
    if (params?.offset) q.set('offset', String(params.offset));
    return api.get<{ data: MediaAsset[]; meta: { total: number } }>(`/media?${q.toString()}`);
  },

  upload: (file: File, altText = '', caption = '') => {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('alt_text', altText);
    formData.append('caption', caption);
    return api.upload<{ data: MediaAsset }>('/media', formData);
  },

  delete: (id: string) => api.delete(`/media/${id}`),

  getSignedURL: (id: string, ttl = 3600) =>
    api.get<{ signed_url: string; expires_in: number }>(`/media/${id}/signed-url?ttl=${ttl}`),
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
  list: () => api.get<{ data: User[] }>('/users'),
};

