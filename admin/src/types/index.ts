export interface User {
  id: string;
  username: string;
  email: string;
  first_name?: string;
  last_name?: string;
  status: 'active' | 'inactive' | 'suspended';
  roles?: Role[];
  last_login_at?: string;
  mfa_enabled?: boolean;
  sso_provider?: string;
  failed_login_attempts?: number;
  locked_until?: string | null;
  created_at?: string;
  updated_at?: string;
}

export interface Permission {
  id?: string;
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
  is_system?: boolean;
  user_count?: number;
  created_at?: string;
  updated_at?: string;
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
  | 'time'
  | 'blocks'
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
  | 'dynamic_zone'
  | 'uid';

export interface ValidationRules {
  // Keep extension keys when editing schemas created by newer servers/plugins.
  [key: string]: unknown;
  description?: string;
  placeholder?: string;
  default?: unknown;
  group?: string;
  hidden?: boolean;
  read_only?: boolean;
  private?: boolean;
  read_roles?: string[];
  edit_roles?: string[];
  error_message?: string;
  allowed_protocols?: string[];
  date_min?: string;
  date_max?: string;
  timezone?: string;
  allowed_blocks?: string[];
  json_schema?: Record<string, unknown>;
  relation_cardinality?: 'many_to_one' | 'many_to_many';
  layout_width?: 'full' | 'half' | 'third' | 'two-thirds';
  required?: boolean;
  unique?: boolean;
  min_length?: number;
  max_length?: number;
  min?: number;
  max?: number;
  regex?: string;
  enum_values?: string[];
  target_field?: string;
  target_type?: string;
  multiple?: boolean;
  allowed_types?: string[];
  min_items?: number;
  max_items?: number;
  component?: string;
  repeatable?: boolean;
  components?: string[];
  preserve_order?: boolean;
}

export interface ContentField {
  id?: string;
  name: string;
  field_key: string;
  field_type: FieldType;
  configuration: ValidationRules;
}

export type ContentKind = 'collection' | 'single' | 'component';

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
  minimumCmsVersion?: string;
  runtime: string;
  permissions?: string[];
  settingsSchema?: Record<string, PluginSettingSchemaField>;
  routes?: Array<{ method: string; path: string; permission?: string }>;
}

export interface PluginSettingSchemaField {
  type: 'boolean' | 'number' | 'string' | string;
  label?: string;
  description?: string;
  default?: any;
  min?: number;
  max?: number;
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

export interface PublicGeneralSettings {
  site_name: string;
  site_description: string;
  public_url: string;
  default_timezone: string;
  default_locale: string;
  date_format: string;
  time_format: string;
  logo_url: string;
  favicon_url: string;
}

export interface GeneralSettings extends PublicGeneralSettings {
  admin_path: string;
  logo_media_id: string;
  favicon_media_id: string;
  updated_at?: string;
}

export interface UserPreferences {
  user_id?: string;
  theme: 'system' | 'light' | 'dark';
  preferred_language: string;
  timezone: string;
  date_format: string;
  time_format: string;
  sidebar_collapsed: boolean;
  updated_at?: string;
}

export interface SettingsOverview {
  site_name: string;
  public_url: string;
  environment: string;
  cms_version: string;
  database_driver: string;
  database_status: string;
  storage_provider: string;
  cache_driver: string;
  jobs_enabled: boolean;
  plugins_total: number;
  plugins_enabled: number;
  themes_count: number;
  active_theme: string;
  default_locale: string;
  signed_urls_active: boolean;
}

export interface MigrationItem {
  version: string;
  name: string;
  applied: boolean;
  applied_at?: string;
}

export interface SystemInfo {
  application: {
    name: string;
    version: string;
    environment: string;
    started_at: string;
    uptime_seconds: number;
    go_version: string;
    os: string;
    arch: string;
    goroutines: number;
    host: string;
    port: number;
  };
  environment_config: {
    read_only: boolean;
    logging_level: string;
    logging_format: string;
    cors_origins_count: number;
    trusted_proxies: string[];
    token_expiration: string;
  };
  database: {
    driver: string;
    status: string;
    host: string;
    port: number;
    database: string;
    ssl_mode: string;
    max_open_conns: number;
    max_idle_conns: number;
    applied_migrations: number;
    pending_migrations: number;
    migrations?: MigrationItem[];
    connection_note: string;
    error?: string;
  };
  storage: {
    provider: string;
    upload_dir: string;
    signed_urls_enabled: boolean;
    signed_url_algorithm: string;
    signed_url_expiration: string;
  };
  cache: {
    driver: string;
    default_ttl: string;
    prefix: string;
    status: string;
  };
  jobs: {
    enabled: boolean;
    concurrency: number;
    max_retries: number;
    poll_interval: string;
    queue_stats?: Record<string, number>;
  };
  themes: {
    active_theme: string;
    themes_dir: string;
    cache: boolean;
    installed?: string[];
  };
  plugins: {
    plugins_dir: string;
    total: number;
    enabled: number;
  };
}


export interface AdminInvitation {
  id: string;
  email: string;
  username: string;
  first_name?: string;
  last_name?: string;
  role_ids: string[];
  roles?: Role[];
  status: 'pending' | 'accepted' | 'revoked' | 'expired';
  invited_by?: string;
  expires_at: string;
  accepted_at?: string;
  created_at: string;
  updated_at: string;
}

export interface InvitationPreview {
  email: string;
  username: string;
  first_name?: string;
  last_name?: string;
  expires_at: string;
  roles?: Role[];
}

export type TokenKind = 'api' | 'admin' | 'transfer';

export interface ApiToken {
  id: string;
  name: string;
  kind: TokenKind;
  description?: string;
  token_type: string;
  token_prefix: string;
  user_id: string;
  permissions: string[];
  expires_at?: string;
  last_used_at?: string;
  revoked_at?: string;
  created_at: string;
}

export interface CreateTokenPayload {
  name: string;
  kind: TokenKind;
  description?: string;
  token_type: string;
  permissions?: string[];
  duration_seconds?: number;
  never_expires?: boolean;
}

export interface RestAPISettings {
  default_limit: number;
  max_limit: number;
  with_count: boolean;
  max_populate_depth: number;
  expose_draft_preview: boolean;
  updated_at?: string;
}

export interface CORSSettings {
  enabled: boolean;
  allowed_origins: string[];
  allowed_methods: string[];
  allowed_headers: string[];
  exposed_headers: string[];
  allow_credentials: boolean;
  max_age_seconds: number;
  updated_at?: string;
}

export interface RateLimitSettings {
  enabled: boolean;
  global_requests_per_sec: number;
  global_burst: number;
  auth_requests_per_min: number;
  auth_burst: number;
  updated_at?: string;
}


export interface MediaSettings {
  max_file_size_bytes: number;
  allowed_mime_types: string[];
  responsive_sizes_enabled: boolean;
  generate_small: boolean;
  generate_medium: boolean;
  generate_large: boolean;
  require_alt_text: boolean;
  default_folder_id?: string | null;
  default_access: 'public' | 'signed';
  signed_url_ttl_seconds: number;
  updated_at?: string;
}

export interface StorageProviderCapability {
  id: string;
  label: string;
  implemented: boolean;
  reason?: string;
}

export interface LocalStorageSettings {
  upload_dir: string;
  url_prefix: string;
}

export interface S3StorageSettings {
  bucket: string;
  region: string;
  endpoint: string;
  access_key_id: string;
  secret_access_key?: string;
  secret_configured: boolean;
  use_ssl: boolean;
  force_path_style: boolean;
  base_url: string;
}

export interface StorageTestResult {
  ok: boolean;
  provider: string;
  latency_ms: number;
  message: string;
}

export interface StorageSettings {
  active_provider: 'local' | 's3' | 'minio';
  local: LocalStorageSettings;
  s3: S3StorageSettings;
  supported_providers?: StorageProviderCapability[];
  updated_at?: string;
}

export interface EmailSettings {
  provider: 'smtp' | 'log' | 'disabled';
  from_name: string;
  from_email: string;
  default_reply_to: string;
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password?: string;
  password_configured: boolean;
  encryption: 'tls' | 'starttls' | 'none';
  skip_tls_verify: boolean;
  timeout_seconds: number;
  updated_at?: string;
}

export interface EmailTestResult {
  ok: boolean;
  provider: string;
  latency_ms: number;
  message: string;
}

export interface EmailTemplate {
  id: string;
  template_key: string;
  name: string;
  subject: string;
  from_name: string;
  from_email: string;
  reply_to: string;
  body_html: string;
  body_text: string;
  variables: string[];
  enabled: boolean;
  updated_at: string;
}

export interface UpdateEmailTemplateInput {
  name: string;
  subject: string;
  from_name: string;
  from_email: string;
  reply_to: string;
  body_html: string;
  body_text: string;
  enabled: boolean;
}

export interface EmailTemplatePreview {
  subject: string;
  body_html: string;
  body_text: string;
}


export interface SecuritySettings {
  session_timeout_minutes: number;
  max_failed_logins: number;
  lockout_duration_minutes: number;
  enforce_admin_mfa: boolean;
  allowed_admin_ip_cidrs: string[];
  updated_at?: string;
}

export interface PasswordPolicySettings {
  min_length: number;
  require_uppercase: boolean;
  require_lowercase: boolean;
  require_number: boolean;
  require_special: boolean;
  max_age_days: number;
  updated_at?: string;
}

export interface SSOSettings {
  enabled: boolean;
  auto_provision_admin: boolean;
  default_role_id: string;
  allowed_domains: string[];
  enforce_sso_for_domains: boolean;
  updated_at?: string;
}

export interface LoginResponse {
  token?: string;
  user: User;
  mfa_required?: boolean;
  mfa_setup_required?: boolean;
  mfa_token?: string;
}

export interface MFAEnrollResult {
  secret: string;
  otpauth_uri: string;
  issuer: string;
  account: string;
}

export interface MFAConfirmResult {
  user: User;
  recovery_codes: string[];
  token?: string;
}

export interface AppRole {
  id: string;
  name: string;
  slug: string;
  description: string;
  type: 'authenticated' | 'public' | 'custom' | string;
  is_default: boolean;
  permissions: Record<string, Record<string, boolean>>;
  user_count?: number;
  created_at: string;
  updated_at: string;
}

export interface AppUser {
  id: string;
  username: string;
  email: string;
  provider: string;
  confirmed: boolean;
  blocked: boolean;
  role_id: string;
  role?: AppRole;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
}

export interface OAuthProvider {
  id: string;
  name: string;
  display_name: string;
  provider_type: string;
  scope: 'app' | 'admin' | string;
  enabled: boolean;
  client_id: string;
  client_secret?: string;
  secret_configured: boolean;
  auth_url: string;
  token_url: string;
  userinfo_url: string;
  issuer_url: string;
  redirect_uri: string;
  scopes: string;
  created_at: string;
  updated_at: string;
}

export interface PublicProviderInfo {
  name: string;
  display_name: string;
  provider_type: string;
  scope: string;
}

export interface AppAuthSettings {
  allow_register: boolean;
  email_confirmation: boolean;
  reset_password_url: string;
  email_confirmation_redirection_url: string;
  default_role_id: string;
  session_expiration_minutes: number;
  updated_at?: string;
}

export interface OIDCDiscoveryResult {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  userinfo_endpoint: string;
  scopes_supported?: string[];
}

export interface WebhookEventDescription {
  name: string;
  label: string;
  category: string;
  description: string;
}

export interface Webhook {
  id: string;
  name: string;
  url: string;
  headers: Record<string, string>;
  events: string[];
  secret?: string;
  secret_configured: boolean;
  is_enabled: boolean;
  max_retries: number;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface WebhookDelivery {
  id: string;
  webhook_id: string;
  event: string;
  request_url: string;
  status_code: number;
  duration_ms: number;
  attempt: number;
  success: boolean;
  error_message?: string;
  response_snippet?: string;
  created_at: string;
}

export interface CreateWebhookPayload {
  name: string;
  url: string;
  headers: Record<string, string>;
  events: string[];
  secret?: string;
  is_enabled: boolean;
  max_retries: number;
}

export interface UpdateWebhookPayload {
  name?: string;
  url?: string;
  headers?: Record<string, string>;
  events?: string[];
  secret?: string;
  is_enabled?: boolean;
  max_retries?: number;
}

export interface Locale {
  id: string;
  code: string;
  name: string;
  native_name: string;
  is_default: boolean;
  is_enabled: boolean;
  fallback_code?: string;
  direction: 'ltr' | 'rtl' | string;
  created_at: string;
  updated_at: string;
}

export interface CreateLocalePayload {
  code: string;
  name: string;
  native_name?: string;
  is_default?: boolean;
  is_enabled?: boolean;
  fallback_code?: string;
  direction: 'ltr' | 'rtl';
}

export interface UpdateLocalePayload {
  name?: string;
  native_name?: string;
  is_default?: boolean;
  is_enabled?: boolean;
  fallback_code?: string;
  direction?: 'ltr' | 'rtl';
}

export interface AuditLogEntry {
  id: string;
  actor_id: string;
  actor_type: string;
  action: string;
  resource: string;
  target_id: string;
  metadata?: Record<string, unknown>;
  ip_address: string;
  user_agent?: string;
  result?: string;
  created_at: string;
}

export interface AuditLogFilter {
  actor_id?: string;
  action?: string;
  resource?: string;
  result?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}
