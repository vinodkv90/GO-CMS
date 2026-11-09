export interface User {
  id: string;
  username: string;
  email: string;
  first_name?: string;
  last_name?: string;
  status: 'active' | 'inactive' | 'suspended';
  roles?: Role[];
}

export interface Permission {
  action: string;
  resource: string;
  description?: string;
}

export interface Role {
  id: string;
  name: string;
  slug: string;
  description?: string;
  permissions?: Permission[];
}

export type FieldType =
  | 'short_text'
  | 'long_text'
  | 'rich_text'
  | 'number'
  | 'decimal'
  | 'boolean'
  | 'date'
  | 'datetime'
  | 'email'
  | 'secret'
  | 'url'
  | 'enum'
  | 'json'
  | 'media'
  | 'relation_single'
  | 'relation_multiple'
  | 'component'
  | 'repeatable_component'
  | 'uid';

export interface ValidationRules {
  required?: boolean;
  unique?: boolean;
  min_length?: number;
  max_length?: number;
  min?: number;
  max?: number;
  regex?: string;
  enum_values?: string[];
}

export interface ContentField {
  id?: string;
  name: string;
  field_key: string;
  field_type: FieldType;
  configuration: ValidationRules;
}

export type ContentKind = 'collection' | 'single';

export interface ContentType {
  id: string;
  uid: string;
  name: string;
  kind: ContentKind;
  settings?: Record<string, any>;
  fields?: ContentField[];
  created_at?: string;
  updated_at?: string;
}

export type ContentStatus = 'draft' | 'published' | 'archived';

export interface ContentEntry {
  id: string;
  content_type_id: string;
  slug?: string;
  draft_data: Record<string, any>;
  published_data?: Record<string, any>;
  status: ContentStatus;
  draft_version: number;
  published_version: number;
  created_at: string;
  updated_at: string;
  published_at?: string;
}

export interface ContentVersion {
  id: string;
  entry_id: string;
  version_number: number;
  snapshot: Record<string, any>;
  action: string;
  changed_by?: string;
  created_at: string;
}

export interface MediaFolder {
  id: string;
  name: string;
  slug: string;
  parent_id?: string | null;
  path: string;
  storage_path: string;
  created_at: string;
  updated_at: string;
  created_by?: string | null;
}

export interface MediaFolderTreeNode extends MediaFolder {
  children: MediaFolderTreeNode[];
  asset_count: number;
}

export interface MediaAsset {
  id: string;
  folder_id?: string | null;
  filename: string;
  original_name: string;
  mime_type: string;
  file_size: number;
  storage_provider: string;
  storage_key: string;
  url: string;
  metadata?: {
    width?: number;
    height?: number;
    alt_text?: string;
    caption?: string;
    format?: string;
    thumbnails?: Record<string, string>;
    custom?: Record<string, any>;
  };
  created_at: string;
}

export interface PluginManifest {
  id: string;
  name: string;
  version: string;
  description: string;
  author: string;
  runtime: string;
  permissions?: string[];
  routes?: Array<{ method: string; path: string }>;
}

export interface PluginEntity {
  manifest: PluginManifest;
  status: 'discovered' | 'installed' | 'enabled' | 'disabled' | 'error';
  settings?: Record<string, any>;
  enabled_at?: string;
  error_msg?: string;
}

export interface ThemeManifest {
  name: string;
  slug: string;
  version: string;
  description: string;
  author: string;
  engine: string;
  settings?: Record<string, any>;
  is_active?: boolean;
}

