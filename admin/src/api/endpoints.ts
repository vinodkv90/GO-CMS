import { api } from './client';
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
  list: (typeUID: string, params?: { limit?: number; offset?: number; preview?: boolean; search?: string }) => {
    const q = new URLSearchParams();
    if (params?.limit) q.set('limit', String(params.limit));
    if (params?.offset) q.set('offset', String(params.offset));
    if (params?.preview) q.set('preview', 'true');
    if (params?.search) q.set('search', params.search);
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
  list: () => api.get<{ data: User[] }>('/users'),
};
