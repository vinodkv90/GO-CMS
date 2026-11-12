import React, { useCallback, useRef, useState } from 'react';
import {
  Activity,
  ArrowLeftRight,
  Cloud,
  Database,
  FileCode2,
  Gauge,
  Globe,
  HardDrive,
  Image,
  Info,
  KeyRound,
  Languages,
  Layers,
  Lock,
  Mail,
  Puzzle,
  Server,
  Shield,
  ShieldCheck,
  Sliders,
  SlidersHorizontal,
  User as UserIcon,
  UserCheck,
  Users,
  Webhook,
  ClipboardList,
} from 'lucide-react';
import {
  SettingsLayout,
  type SettingsNavGroup,
} from '../components/settings/SettingsLayout';
import { SettingsOverviewView } from './settings/SettingsOverviewView';
import { GeneralSettingsView } from './settings/GeneralSettingsView';
import { MediaSettingsView } from './settings/MediaSettingsView';
import { StorageProvidersView } from './settings/StorageProvidersView';
import { EmailSettingsView } from './settings/EmailSettingsView';
import { EmailTemplatesView } from './settings/EmailTemplatesView';
import { AdminUsersSettingsView } from './settings/AdminUsersSettingsView';
import { AdminRolesSettingsView } from './settings/AdminRolesSettingsView';
import { SecurityAuthSettingsView } from './settings/SecurityAuthSettingsView';
import { SsoSettingsView } from './settings/SsoSettingsView';
import { AuditLogsSettingsView } from './settings/AuditLogsSettingsView';
import { UsersPermissionsRolesView } from './settings/UsersPermissionsRolesView';
import { UsersPermissionsUsersView } from './settings/UsersPermissionsUsersView';
import { UsersPermissionsProvidersView } from './settings/UsersPermissionsProvidersView';
import { UsersPermissionsAdvancedView } from './settings/UsersPermissionsAdvancedView';
import { WebhooksSettingsView } from './settings/WebhooksSettingsView';
import { PluginSettingsView } from './settings/PluginSettingsView';
import { InternationalizationSettingsView } from './settings/InternationalizationSettingsView';
import { ApiTokensView } from './settings/ApiTokensView';
import { TransferTokensView } from './settings/TransferTokensView';
import { RestApiSettingsView } from './settings/RestApiSettingsView';
import { CorsSettingsView } from './settings/CorsSettingsView';
import { RateLimitingView } from './settings/RateLimitingView';
import { ProfileSettingsView } from './settings/ProfileSettingsView';
import {
  SystemSettingsView,
  type SystemSubSection,
} from './settings/SystemSettingsView';
import { Alert } from '../components/ui/Alert';
import { hasPermission } from '../utils/rbac';
import type { PublicGeneralSettings, User, UserPreferences } from '../types';

export interface SettingsViewProps {
  user: User;
  section: string;
  onSelectSection: (section: string) => void;
  onUserUpdated: (user: User) => void;
  onPublicUpdated: (pub: PublicGeneralSettings) => void;
  onPreferencesUpdated?: (prefs: UserPreferences) => void;
}

const SECTION_META: Record<
  string,
  { title: string; description: string; groupLabel: string }
> = {
  overview: {
    title: 'Overview',
    description: 'Platform configuration summary and health indicators.',
    groupLabel: 'Global Settings',
  },
  general: {
    title: 'General',
    description: 'Configure site identity, public URL, regional defaults, and branding assets.',
    groupLabel: 'Global Settings',
  },
  media: {
    title: 'Media Library',
    description: 'Enforce upload file size limits, allowed MIME types, responsive thumbnails, and signed access.',
    groupLabel: 'Global Settings',
  },
  email: {
    title: 'Email Configuration',
    description: 'Configure SMTP / Log outbound transport, sender identity, and connectivity verification.',
    groupLabel: 'Global Settings',
  },
  i18n: {
    title: 'Internationalization',
    description: 'Manage supported locales, fallback chains, default language, and LTR/RTL layout directions.',
    groupLabel: 'Global Settings',
  },
  webhooks: {
    title: 'Webhooks',
    description: 'Manage SSRF-protected outgoing HTTP webhooks, HMAC-SHA256 secrets, and delivery logs.',
    groupLabel: 'Global Settings',
  },
  plugins: {
    title: 'Plugins',
    description: 'Enable, disable, and configure CMS plugins with persistent database state.',
    groupLabel: 'Global Settings',
  },
  'email-templates': {
    title: 'Email Templates',
    description: 'Customize sanitized transactional email templates and preview rendered output.',
    groupLabel: 'Users & Permissions',
  },
  'admin-users': {
    title: 'Administration Users',
    description: 'Invite administrators, manage role assignments, and control account activation.',
    groupLabel: 'Administration Panel',
  },
  'admin-roles': {
    title: 'Administration Roles',
    description: 'Create custom roles and configure fine-grained RBAC permissions.',
    groupLabel: 'Administration Panel',
  },
  security: {
    title: 'Security & Authentication',
    description: 'Configure session lifetimes, brute-force lockout, password policy, MFA enforcement, and IP allowlisting.',
    groupLabel: 'Administration Panel',
  },
  sso: {
    title: 'Single Sign-On',
    description: 'Configure Enterprise OIDC SSO identity providers, auto-provisioning, and email domain enforcement.',
    groupLabel: 'Administration Panel',
  },
  'audit-logs': {
    title: 'Audit Logs',
    description: 'Inspect security, authentication, RBAC, settings, and plugin activity with automatic secret redaction and CSV export.',
    groupLabel: 'Administration Panel',
  },
  'up-roles': {
    title: 'End-User Roles',
    description: 'Manage permissions for public visitors and authenticated application end-users.',
    groupLabel: 'Users & Permissions',
  },
  'up-users': {
    title: 'Application Users',
    description: 'Create, confirm, block, and assign roles to application end-users separately from admin accounts.',
    groupLabel: 'Users & Permissions',
  },
  'up-providers': {
    title: 'Authentication Providers',
    description: 'Configure OAuth2 and OpenID Connect providers for end-user login.',
    groupLabel: 'Users & Permissions',
  },
  'up-advanced': {
    title: 'Advanced Settings',
    description: 'Control self-registration, unique emails, confirmation flow, and default end-user role.',
    groupLabel: 'Users & Permissions',
  },
  'api-tokens': {
    title: 'API Tokens',
    description: 'Manage scoped Content API tokens and Admin Personal Access Tokens.',
    groupLabel: 'API Configuration',
  },
  'transfer-tokens': {
    title: 'Transfer Tokens',
    description: 'Create scoped migration tokens restricted strictly to data push/pull transfers.',
    groupLabel: 'API Configuration',
  },
  'rest-api': {
    title: 'REST API Settings',
    description: 'Configure default pagination limits, populate depth caps, and draft preview exposure.',
    groupLabel: 'API Configuration',
  },
  cors: {
    title: 'CORS Settings',
    description: 'Manage allowed origins, HTTP methods, headers, and credential policies live.',
    groupLabel: 'API Configuration',
  },
  'rate-limiting': {
    title: 'Rate Limiting',
    description: 'Tune global and authentication token-bucket throttling live without server restart.',
    groupLabel: 'API Configuration',
  },
  profile: {
    title: 'Profile & Preferences',
    description: 'Update your administrator profile, password, and interface preferences.',
    groupLabel: 'Account',
  },
  system: {
    title: 'System Information',
    description: 'Runtime environment, database status, storage, themes, and extensions.',
    groupLabel: 'System',
  },
  'system-environment': {
    title: 'Environment',
    description: 'Read-only server startup configuration and security boundaries.',
    groupLabel: 'System',
  },
  'system-database': {
    title: 'Database',
    description: 'Connection status, pooling limits, and schema migration history.',
    groupLabel: 'System',
  },
  'system-cache': {
    title: 'Cache',
    description: 'Active caching driver and TTL policies.',
    groupLabel: 'System',
  },
  'system-jobs': {
    title: 'Background Jobs',
    description: 'Worker pool configuration and live queue counts.',
    groupLabel: 'System',
  },
  'storage-providers': {
    title: 'Storage Providers',
    description: 'Switch between Local Disk, Amazon S3, and MinIO object storage and test connectivity.',
    groupLabel: 'System',
  },
};

export const SettingsView: React.FC<SettingsViewProps> = ({
  user,
  section,
  onSelectSection,
  onUserUpdated,
  onPublicUpdated,
  onPreferencesUpdated,
}) => {
  const canReadSettings = hasPermission(user, 'read', 'settings');
  const canUpdateSettings = hasPermission(user, 'update', 'settings');
  const canReadUsers = hasPermission(user, 'read', 'users');
  const canReadRoles = hasPermission(user, 'read', 'roles');
  const canReadTokens = hasPermission(user, 'read', 'api_tokens');
  const canReadSystem = hasPermission(user, 'read', 'system:info');
  const canReadAuditLogs = hasPermission(user, 'read', 'system:audit_logs');
  const canReadUsersPerm = hasPermission(user, 'read', 'users_permissions');
  const canUpdateUsersPerm = hasPermission(user, 'update', 'users_permissions');
  const canReadWebhooks = hasPermission(user, 'read', 'webhooks');
  const canCreateWebhooks = hasPermission(user, 'create', 'webhooks');
  const canUpdateWebhooks = hasPermission(user, 'update', 'webhooks');
  const canDeleteWebhooks = hasPermission(user, 'delete', 'webhooks');
  const canReadPlugins = hasPermission(user, 'read', 'plugins');
  const canUpdatePlugins = hasPermission(user, 'update', 'plugins');
  const canReadLocales = hasPermission(user, 'read', 'system:locales') || canReadSettings;
  const canCreateLocales = hasPermission(user, 'create', 'system:locales');
  const canUpdateLocales = hasPermission(user, 'update', 'system:locales');
  const canDeleteLocales = hasPermission(user, 'delete', 'system:locales');

  const fallbackSection = canReadSettings
    ? 'overview'
    : canReadTokens
      ? 'api-tokens'
      : canReadUsers
        ? 'admin-users'
        : canReadRoles
          ? 'admin-roles'
          : canReadUsersPerm
            ? 'up-roles'
            : 'profile';
  const activeSection = SECTION_META[section] ? section : fallbackSection;
  const meta = SECTION_META[activeSection];

  const [isDirty, setIsDirty] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const handlersRef = useRef<{
    save: () => Promise<void>;
    discard: () => void;
  } | null>(null);

  const registerHandlers = useCallback(
    (h: { save: () => Promise<void>; discard: () => void; isSaving: boolean }) => {
      handlersRef.current = { save: h.save, discard: h.discard };
      setIsSaving(h.isSaving);
    },
    [],
  );

  const navGroups: SettingsNavGroup[] = [
    {
      id: 'global',
      label: 'Global Settings',
      items: [
        ...(canReadSettings
          ? [
              { id: 'overview', label: 'Overview', icon: <Info className="w-3.5 h-3.5" /> },
              { id: 'general', label: 'General', icon: <Globe className="w-3.5 h-3.5" /> },
            ]
          : []),
        ...(canReadLocales
          ? [{ id: 'i18n', label: 'Internationalization', icon: <Languages className="w-3.5 h-3.5" /> }]
          : []),
        ...(canReadSettings
          ? [
              { id: 'media', label: 'Media Library', icon: <Image className="w-3.5 h-3.5" /> },
              { id: 'email', label: 'Email Configuration', icon: <Mail className="w-3.5 h-3.5" /> },
            ]
          : []),
        ...(canReadWebhooks
          ? [{ id: 'webhooks', label: 'Webhooks', icon: <Webhook className="w-3.5 h-3.5" /> }]
          : []),
        ...(canReadPlugins
          ? [{ id: 'plugins', label: 'Plugins', icon: <Puzzle className="w-3.5 h-3.5" /> }]
          : []),
      ],
    },
    {
      id: 'administration',
      label: 'Administration Panel',
      items: [
        ...(canReadRoles
          ? [{ id: 'admin-roles', label: 'Roles', icon: <Shield className="w-3.5 h-3.5" /> }]
          : []),
        ...(canReadUsers
          ? [{ id: 'admin-users', label: 'Users', icon: <Users className="w-3.5 h-3.5" /> }]
          : []),
        ...(canReadSettings
          ? [
              { id: 'security', label: 'Security & Auth', icon: <Lock className="w-3.5 h-3.5" /> },
              { id: 'sso', label: 'Single Sign-On', icon: <ShieldCheck className="w-3.5 h-3.5" /> },
            ]
          : []),
        ...(canReadAuditLogs
          ? [{ id: 'audit-logs', label: 'Audit Logs', icon: <ClipboardList className="w-3.5 h-3.5" /> }]
          : []),
      ],
    },
    {
      id: 'users-permissions',
      label: 'Users & Permissions',
      items: [
        ...(canReadUsersPerm
          ? [
              { id: 'up-roles', label: 'Roles', icon: <Shield className="w-3.5 h-3.5" /> },
              { id: 'up-users', label: 'Users', icon: <UserCheck className="w-3.5 h-3.5" /> },
              { id: 'up-providers', label: 'Providers', icon: <Globe className="w-3.5 h-3.5" /> },
            ]
          : []),
        ...(canReadSettings
          ? [
              {
                id: 'email-templates',
                label: 'Email Templates',
                icon: <FileCode2 className="w-3.5 h-3.5" />,
              },
            ]
          : []),
        ...(canReadUsersPerm
          ? [
              { id: 'up-advanced', label: 'Advanced Settings', icon: <Sliders className="w-3.5 h-3.5" /> },
            ]
          : []),
      ],
    },
    {
      id: 'api-config',
      label: 'API Configuration',
      items: [
        ...(canReadTokens
          ? [
              { id: 'api-tokens', label: 'API Tokens', icon: <KeyRound className="w-3.5 h-3.5" /> },
              {
                id: 'transfer-tokens',
                label: 'Transfer Tokens',
                icon: <ArrowLeftRight className="w-3.5 h-3.5" />,
              },
            ]
          : []),
        ...(canReadSettings
          ? [
              {
                id: 'rest-api',
                label: 'REST API Settings',
                icon: <SlidersHorizontal className="w-3.5 h-3.5" />,
              },
              { id: 'cors', label: 'CORS Settings', icon: <Globe className="w-3.5 h-3.5" /> },
              {
                id: 'rate-limiting',
                label: 'Rate Limiting',
                icon: <Gauge className="w-3.5 h-3.5" />,
              },
            ]
          : []),
      ],
    },
    {
      id: 'account',
      label: 'Account',
      items: [
        {
          id: 'profile',
          label: 'Profile & Preferences',
          icon: <UserIcon className="w-3.5 h-3.5" />,
        },
      ],
    },
    {
      id: 'system',
      label: 'System',
      items: [
        ...(canReadSettings
          ? [
              {
                id: 'storage-providers',
                label: 'Storage Providers',
                icon: <Cloud className="w-3.5 h-3.5" />,
              },
            ]
          : []),
        ...(canReadSystem
          ? [
              { id: 'system', label: 'System Information', icon: <Server className="w-3.5 h-3.5" /> },
              {
                id: 'system-environment',
                label: 'Environment',
                icon: <HardDrive className="w-3.5 h-3.5" />,
                badge: 'Read-only',
              },
              { id: 'system-database', label: 'Database', icon: <Database className="w-3.5 h-3.5" /> },
              { id: 'system-cache', label: 'Cache', icon: <Layers className="w-3.5 h-3.5" /> },
              { id: 'system-jobs', label: 'Background Jobs', icon: <Activity className="w-3.5 h-3.5" /> },
            ]
          : []),
      ],
    },
  ];

  const handleDirtyChange = useCallback((dirty: boolean) => {
    setIsDirty(dirty);
  }, []);

  const handleSelectSection = (next: string) => {
    setIsDirty(false);
    handlersRef.current = null;
    onSelectSection(next);
  };

  let body: React.ReactNode = null;
  const showsSaveBar =
    activeSection === 'general' ||
    activeSection === 'media' ||
    activeSection === 'email' ||
    activeSection === 'storage-providers' ||
    activeSection === 'security' ||
    activeSection === 'up-advanced' ||
    activeSection === 'rest-api' ||
    activeSection === 'cors' ||
    activeSection === 'rate-limiting';

  if (activeSection === 'overview') {
    body = canReadSettings ? (
      <SettingsOverviewView onNavigateSection={handleSelectSection} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view global settings.
      </Alert>
    );
  } else if (activeSection === 'general') {
    body = canReadSettings ? (
      <GeneralSettingsView
        canEdit={canUpdateSettings}
        onDirtyChange={handleDirtyChange}
        registerHandlers={registerHandlers}
        onPublicUpdated={onPublicUpdated}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view global settings.
      </Alert>
    );
  } else if (activeSection === 'media') {
    body = canReadSettings ? (
      <MediaSettingsView
        canEdit={canUpdateSettings}
        onDirtyChange={handleDirtyChange}
        registerHandlers={registerHandlers}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view media library settings.
      </Alert>
    );
  } else if (activeSection === 'email') {
    body = canReadSettings ? (
      <EmailSettingsView
        canEdit={canUpdateSettings}
        onDirtyChange={handleDirtyChange}
        registerHandlers={registerHandlers}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view email settings.
      </Alert>
    );
  } else if (activeSection === 'email-templates') {
    body = canReadSettings ? (
      <EmailTemplatesView canEdit={canUpdateSettings} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view email templates.
      </Alert>
    );
  } else if (activeSection === 'storage-providers') {
    body = canReadSettings ? (
      <StorageProvidersView
        canEdit={canUpdateSettings}
        onDirtyChange={handleDirtyChange}
        registerHandlers={registerHandlers}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view storage provider settings.
      </Alert>
    );
  } else if (activeSection === 'admin-users') {
    body = canReadUsers ? (
      <AdminUsersSettingsView
        currentUser={user}
        onUserUpdated={onUserUpdated}
        onNavigateRoles={canReadRoles ? () => handleSelectSection('admin-roles') : undefined}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view administrator accounts.
      </Alert>
    );
  } else if (activeSection === 'admin-roles') {
    body = canReadRoles ? (
      <AdminRolesSettingsView currentUser={user} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view administration roles.
      </Alert>
    );
  } else if (activeSection === 'security') {
    body = canReadSettings ? (
      <SecurityAuthSettingsView
        canEdit={canUpdateSettings}
        onDirtyChange={handleDirtyChange}
        registerHandlers={registerHandlers}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view security and authentication settings.
      </Alert>
    );
  } else if (activeSection === 'audit-logs') {
    body = canReadAuditLogs ? (
      <AuditLogsSettingsView />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view Audit Logs.
      </Alert>
    );
  } else if (activeSection === 'sso') {
    body = canReadSettings ? (
      <SsoSettingsView canEdit={canUpdateSettings} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view Single Sign-On settings.
      </Alert>
    );
  } else if (activeSection === 'up-roles') {
    body = canReadUsersPerm ? (
      <UsersPermissionsRolesView canEdit={canUpdateUsersPerm} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view end-user roles.
      </Alert>
    );
  } else if (activeSection === 'up-users') {
    body = canReadUsersPerm ? (
      <UsersPermissionsUsersView canEdit={canUpdateUsersPerm} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view application users.
      </Alert>
    );
  } else if (activeSection === 'up-providers') {
    body = canReadUsersPerm ? (
      <UsersPermissionsProvidersView canEdit={canUpdateUsersPerm} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view authentication providers.
      </Alert>
    );
  } else if (activeSection === 'up-advanced') {
    body = canReadUsersPerm ? (
      <UsersPermissionsAdvancedView
        canEdit={canUpdateUsersPerm}
        onDirtyChange={handleDirtyChange}
        registerHandlers={registerHandlers}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view Users & Permissions advanced settings.
      </Alert>
    );
  } else if (activeSection === 'webhooks') {
    body = canReadWebhooks ? (
      <WebhooksSettingsView
        canCreate={canCreateWebhooks}
        canEdit={canUpdateWebhooks}
        canDelete={canDeleteWebhooks}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view outgoing webhooks.
      </Alert>
    );
  } else if (activeSection === 'plugins') {
    body = canReadPlugins ? (
      <PluginSettingsView canEdit={canUpdatePlugins} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view installed plugins.
      </Alert>
    );
  } else if (activeSection === 'i18n') {
    body = canReadLocales ? (
      <InternationalizationSettingsView
        canCreate={canCreateLocales}
        canEdit={canUpdateLocales}
        canDelete={canDeleteLocales}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view internationalization locales.
      </Alert>
    );
  } else if (activeSection === 'api-tokens') {
    body = canReadTokens ? (
      <ApiTokensView currentUser={user} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view API tokens.
      </Alert>
    );
  } else if (activeSection === 'transfer-tokens') {
    body = canReadTokens ? (
      <TransferTokensView currentUser={user} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view transfer tokens.
      </Alert>
    );
  } else if (activeSection === 'rest-api') {
    body = canReadSettings ? (
      <RestApiSettingsView
        canEdit={canUpdateSettings}
        onDirtyChange={handleDirtyChange}
        registerHandlers={registerHandlers}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view REST API settings.
      </Alert>
    );
  } else if (activeSection === 'cors') {
    body = canReadSettings ? (
      <CorsSettingsView
        canEdit={canUpdateSettings}
        onDirtyChange={handleDirtyChange}
        registerHandlers={registerHandlers}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view CORS settings.
      </Alert>
    );
  } else if (activeSection === 'rate-limiting') {
    body = canReadSettings ? (
      <RateLimitingView
        canEdit={canUpdateSettings}
        onDirtyChange={handleDirtyChange}
        registerHandlers={registerHandlers}
      />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view rate limiting settings.
      </Alert>
    );
  } else if (activeSection === 'profile') {
    body = (
      <ProfileSettingsView
        user={user}
        onUserUpdated={onUserUpdated}
        onPreferencesUpdated={onPreferencesUpdated}
      />
    );
  } else if (
    activeSection === 'system' ||
    activeSection === 'system-environment' ||
    activeSection === 'system-database' ||
    activeSection === 'system-cache' ||
    activeSection === 'system-jobs'
  ) {
    body = canReadSystem ? (
      <SystemSettingsView subsection={activeSection as SystemSubSection} />
    ) : (
      <Alert variant="error" title="Permission required">
        You do not have permission to view system diagnostics.
      </Alert>
    );
  }

  return (
    <SettingsLayout
      groups={navGroups}
      activeSection={activeSection}
      onSelectSection={handleSelectSection}
      title={meta.title}
      description={meta.description}
      breadcrumbGroup={meta.groupLabel}
      isDirty={showsSaveBar ? isDirty : false}
      isSaving={isSaving}
      canSave={activeSection === 'up-advanced' ? canUpdateUsersPerm : canUpdateSettings}
      onSave={showsSaveBar ? () => void handlersRef.current?.save() : undefined}
      onDiscard={showsSaveBar ? () => handlersRef.current?.discard() : undefined}
    >
      {body}
    </SettingsLayout>
  );
};
