import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ToastProvider } from '../../components/ui/Toast';
import { SecurityAuthSettingsView } from './SecurityAuthSettingsView';
import { SsoSettingsView } from './SsoSettingsView';
import { UsersPermissionsRolesView } from './UsersPermissionsRolesView';
import { UsersPermissionsUsersView } from './UsersPermissionsUsersView';
import { UsersPermissionsProvidersView } from './UsersPermissionsProvidersView';
import { UsersPermissionsAdvancedView } from './UsersPermissionsAdvancedView';
import { ProfileSettingsView } from './ProfileSettingsView';
import { LoginView } from '../LoginView';
import * as endpoints from '../../api/endpoints';
import type { User } from '../../types';

vi.mock('../../api/endpoints', async () => {
  const actual = await vi.importActual<typeof import('../../api/endpoints')>('../../api/endpoints');
  return {
    ...actual,
    authApi: {
      ...actual.authApi,
      login: vi.fn(),
      verifyMfa: vi.fn(),
      enrollMfa: vi.fn(),
      confirmMfa: vi.fn(),
      disableMfa: vi.fn(),
      regenerateRecoveryCodes: vi.fn(),
      resetExpiredPassword: vi.fn(),
      listSsoProviders: vi.fn(),
    },
    preferencesApi: {
      ...actual.preferencesApi,
      get: vi.fn(),
      update: vi.fn(),
    },
    rolesApi: {
      ...actual.rolesApi,
      list: vi.fn(),
    },
    settingsApi: {
      ...actual.settingsApi,
      getSecurity: vi.fn(),
      updateSecurity: vi.fn(),
      getPasswordPolicy: vi.fn(),
      updatePasswordPolicy: vi.fn(),
      getSso: vi.fn(),
      updateSso: vi.fn(),
    },
    usersPermApi: {
      ...actual.usersPermApi,
      listRoles: vi.fn(),
      createRole: vi.fn(),
      updateRole: vi.fn(),
      deleteRole: vi.fn(),
      listUsers: vi.fn(),
      createUser: vi.fn(),
      updateUser: vi.fn(),
      deleteUser: vi.fn(),
      listProviders: vi.fn(),
      upsertProvider: vi.fn(),
      deleteProvider: vi.fn(),
      discoverOidc: vi.fn(),
      getSettings: vi.fn(),
      updateSettings: vi.fn(),
    },
  };
});

const mockAdminUser: User = {
  id: 'usr-admin-1',
  username: 'admin',
  email: 'admin@example.com',
  status: 'active',
  mfa_enabled: false,
  roles: [{ id: 'role-super', name: 'Super Admin', slug: 'super_admin' }],
};

describe('M5 Security, SSO, MFA & Users-Permissions Views', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders SecurityAuthSettingsView and adds an Admin IP allowlist CIDR', async () => {
    vi.mocked(endpoints.settingsApi.getSecurity).mockResolvedValue({
      data: {
        session_timeout_minutes: 1440,
        max_failed_logins: 5,
        lockout_duration_minutes: 15,
        enforce_admin_mfa: false,
        allowed_admin_ip_cidrs: ['10.0.0.0/8'],
      },
    });
    vi.mocked(endpoints.settingsApi.getPasswordPolicy).mockResolvedValue({
      data: {
        min_length: 10,
        require_uppercase: true,
        require_lowercase: true,
        require_number: true,
        require_special: false,
        max_age_days: 90,
      },
    });

    const onDirtyChange = vi.fn();
    render(
      <ToastProvider>
        <SecurityAuthSettingsView canEdit onDirtyChange={onDirtyChange} registerHandlers={vi.fn()} />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('Allowed Admin IP CIDRs')).toBeInTheDocument());
    expect(screen.getByText('10.0.0.0/8')).toBeInTheDocument();

    const cidrInput = screen.getByPlaceholderText('e.g. 10.0.0.0/8 or 203.0.113.10');
    fireEvent.change(cidrInput, { target: { value: '192.168.1.0/24' } });
    fireEvent.click(screen.getByRole('button', { name: /Add CIDR/i }));

    expect(screen.getByText('192.168.1.0/24')).toBeInTheDocument();
    expect(onDirtyChange).toHaveBeenCalledWith(true);
  });

  it('renders SsoSettingsView, saves SSO policy, and runs OIDC discovery in provider modal', async () => {
    vi.mocked(endpoints.settingsApi.getSso).mockResolvedValue({
      data: {
        enabled: true,
        auto_provision_admin: true,
        default_role_id: '',
        allowed_domains: ['acme.com'],
        enforce_sso_for_domains: false,
      },
    });
    vi.mocked(endpoints.settingsApi.updateSso).mockResolvedValue({
      data: {
        enabled: true,
        auto_provision_admin: true,
        default_role_id: '',
        allowed_domains: ['acme.com'],
        enforce_sso_for_domains: false,
      },
    });
    vi.mocked(endpoints.rolesApi.list).mockResolvedValue({
      data: [{ id: 'role-viewer', name: 'Viewer', slug: 'viewer' }],
    });
    vi.mocked(endpoints.usersPermApi.listProviders).mockResolvedValue({
      data: [
        {
          id: 'prov-1',
          name: 'admin_oidc',
          display_name: 'Okta Enterprise',
          provider_type: 'oidc',
          scope: 'admin',
          enabled: true,
          client_id: 'okta-client-id',
          secret_configured: true,
          issuer_url: 'https://idp.acme.com',
          auth_url: 'https://idp.acme.com/authorize',
          token_url: 'https://idp.acme.com/token',
          userinfo_url: 'https://idp.acme.com/userinfo',
          redirect_uri: '',
          scopes: 'openid email profile',
          created_at: '2026-11-12T00:00:00Z',
          updated_at: '2026-11-12T00:00:00Z',
        },
      ],
    });
    vi.mocked(endpoints.usersPermApi.discoverOidc).mockResolvedValue({
      data: {
        issuer: 'https://idp.acme.com',
        authorization_endpoint: 'https://idp.acme.com/v2/auth',
        token_endpoint: 'https://idp.acme.com/v2/token',
        userinfo_endpoint: 'https://idp.acme.com/v2/userinfo',
      },
    });

    render(
      <ToastProvider>
        <SsoSettingsView canEdit />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('Okta Enterprise')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Save SSO Policy/i }));
    await waitFor(() => expect(endpoints.settingsApi.updateSso).toHaveBeenCalled());

    fireEvent.click(screen.getByRole('button', { name: /^Configure$/i }));
    await waitFor(() => expect(screen.getByRole('button', { name: /Discover OIDC/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Discover OIDC/i }));
    await waitFor(() =>
      expect(screen.getByDisplayValue('https://idp.acme.com/v2/auth')).toBeInTheDocument(),
    );
  });

  it('renders UsersPermissionsRolesView and updates app role permissions', async () => {
    vi.mocked(endpoints.usersPermApi.listRoles).mockResolvedValue({
      data: [
        {
          id: 'app-role-1',
          name: 'Authenticated',
          slug: 'authenticated',
          description: 'Default role for signed-in users',
          type: 'authenticated',
          is_default: true,
          permissions: { content: { find: true, findOne: true } },
          user_count: 3,
          created_at: '2026-11-12T00:00:00Z',
          updated_at: '2026-11-12T00:00:00Z',
        },
      ],
    });
    vi.mocked(endpoints.usersPermApi.updateRole).mockResolvedValue({
      data: {
        id: 'app-role-1',
        name: 'Authenticated',
        slug: 'authenticated',
        description: 'Default role for signed-in users',
        type: 'authenticated',
        is_default: true,
        permissions: { content: { find: true, findOne: true, create: true } },
        created_at: '2026-11-12T00:00:00Z',
        updated_at: '2026-11-12T00:00:00Z',
      },
    });

    render(
      <ToastProvider>
        <UsersPermissionsRolesView canEdit />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('Permissions: Authenticated')).toBeInTheDocument());
    const checkboxes = screen.getAllByRole('checkbox');
    fireEvent.click(checkboxes[2]); // toggle create permission
    fireEvent.click(screen.getByRole('button', { name: /Save Role Permissions/i }));

    await waitFor(() => expect(endpoints.usersPermApi.updateRole).toHaveBeenCalledWith('app-role-1', expect.anything()));
  });

  it('renders UsersPermissionsUsersView, UsersPermissionsProvidersView, and UsersPermissionsAdvancedView', async () => {
    vi.mocked(endpoints.usersPermApi.listRoles).mockResolvedValue({
      data: [
        {
          id: 'app-role-auth',
          name: 'Authenticated',
          slug: 'authenticated',
          description: 'Default role for authenticated end-users',
          type: 'authenticated',
          is_default: true,
          permissions: {},
          created_at: '2026-11-12T00:00:00Z',
          updated_at: '2026-11-12T00:00:00Z',
        },
      ],
    });
    vi.mocked(endpoints.usersPermApi.listUsers).mockResolvedValue({
      data: [
        {
          id: 'au-1',
          username: 'reader1',
          email: 'reader@example.com',
          role_id: 'app-role-auth',
          provider: 'local',
          confirmed: true,
          blocked: false,
          created_at: '2026-11-12T00:00:00Z',
          updated_at: '2026-11-12T00:00:00Z',
        },
      ],
      meta: { total: 1, limit: 50, offset: 0 },
    });
    vi.mocked(endpoints.usersPermApi.listProviders).mockResolvedValue({
      data: [
        {
          id: 'prov-google',
          name: 'google',
          display_name: 'Google OAuth2',
          provider_type: 'oauth2',
          scope: 'app',
          enabled: false,
          client_id: '',
          secret_configured: false,
          issuer_url: '',
          auth_url: '',
          token_url: '',
          userinfo_url: '',
          redirect_uri: '',
          scopes: 'openid email profile',
          created_at: '2026-11-12T00:00:00Z',
          updated_at: '2026-11-12T00:00:00Z',
        },
      ],
    });
    vi.mocked(endpoints.usersPermApi.getSettings).mockResolvedValue({
      data: {
        allow_register: true,
        email_confirmation: false,
        email_confirmation_redirection_url: '',
        default_role_id: 'app-role-auth',
        reset_password_url: '',
        session_expiration_minutes: 1440,
      },
    });

    const { unmount } = render(
      <ToastProvider>
        <UsersPermissionsUsersView canEdit />
      </ToastProvider>,
    );
    await waitFor(() => expect(screen.getByText('reader@example.com')).toBeInTheDocument());
    unmount();

    const provRender = render(
      <ToastProvider>
        <UsersPermissionsProvidersView canEdit />
      </ToastProvider>,
    );
    await waitFor(() => expect(screen.getByText('Google OAuth2')).toBeInTheDocument());
    provRender.unmount();

    render(
      <ToastProvider>
        <UsersPermissionsAdvancedView canEdit onDirtyChange={vi.fn()} registerHandlers={vi.fn()} />
      </ToastProvider>,
    );
    await waitFor(() => expect(screen.getByText('Enable Public End-User Sign-Ups')).toBeInTheDocument());
  });

  it('handles 2FA enrollment and backup recovery code display in ProfileSettingsView', async () => {
    vi.mocked(endpoints.preferencesApi.get).mockResolvedValue({
      data: {
        theme: 'light',
        preferred_language: 'en-US',
        timezone: 'UTC',
        date_format: 'YYYY-MM-DD',
        time_format: '24h',
        sidebar_collapsed: false,
      },
    });
    vi.mocked(endpoints.authApi.enrollMfa).mockResolvedValue({
      data: {
        secret: 'JBSWY3DPEHPK3PXP',
        otpauth_uri: 'otpauth://totp/GoCMS:admin@example.com?secret=JBSWY3DPEHPK3PXP',
        issuer: 'GoCMS',
        account: 'admin@example.com',
      },
    });
    vi.mocked(endpoints.authApi.confirmMfa).mockResolvedValue({
      data: {
        user: { ...mockAdminUser, mfa_enabled: true },
        recovery_codes: ['ABCD-1234', 'EFGH-5678'],
      },
    });

    const onUserUpdated = vi.fn();
    render(
      <ToastProvider>
        <ProfileSettingsView user={mockAdminUser} onUserUpdated={onUserUpdated} />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: /Set Up 2FA/i }));
    await waitFor(() => expect(screen.getByText('JBSWY3DPEHPK3PXP')).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText('123456'), { target: { value: '654321' } });
    fireEvent.click(screen.getByRole('button', { name: /Confirm & Enable 2FA/i }));

    await waitFor(() => expect(screen.getByText('ABCD-1234')).toBeInTheDocument());
    expect(onUserUpdated).toHaveBeenCalledWith(expect.objectContaining({ mfa_enabled: true }));
  });

  it('walks LoginView through multi-step MFA challenge and expired password rotation', async () => {
    vi.mocked(endpoints.authApi.listSsoProviders).mockResolvedValue({
      data: {
        enabled: true,
        providers: [{ name: 'admin_oidc', display_name: 'Corporate SSO', provider_type: 'oidc', scope: 'admin' }],
      },
    });
    vi.mocked(endpoints.authApi.login).mockResolvedValueOnce({
      mfa_required: true,
      mfa_token: 'mfa-challenge-tok',
      user: mockAdminUser,
    });
    vi.mocked(endpoints.authApi.verifyMfa).mockResolvedValueOnce({
      user: mockAdminUser,
      token: 'jwt-ok',
    });

    const onLoginSuccess = vi.fn();
    const first = render(<LoginView onLoginSuccess={onLoginSuccess} />);

    await waitFor(() => expect(screen.getByText(/Continue with Corporate SSO/i)).toBeInTheDocument());

    fireEvent.change(screen.getByPlaceholderText('admin'), { target: { value: 'admin' } });
    fireEvent.change(screen.getByPlaceholderText('••••••••••••'), { target: { value: 'Password123!' } });
    fireEvent.click(screen.getByRole('button', { name: /Sign In/i }));

    await waitFor(() => expect(screen.getByText('Two-Factor Verification')).toBeInTheDocument());
    fireEvent.change(screen.getByPlaceholderText('123456 or ABCD-EFGH'), { target: { value: '123456' } });
    fireEvent.click(screen.getByRole('button', { name: /Verify & Sign In/i }));

    await waitFor(() => expect(onLoginSuccess).toHaveBeenCalledWith(mockAdminUser));
    first.unmount();

    // Test expired password rotation branch (403 ApiError with data.password_expired = true)
    vi.mocked(endpoints.authApi.login).mockRejectedValueOnce(
      Object.assign(new Error('Password expired'), {
        status: 403,
        data: { password_expired: true },
      }),
    );
    vi.mocked(endpoints.authApi.resetExpiredPassword).mockResolvedValueOnce({
      user: mockAdminUser,
      token: 'jwt-rotated',
    });

    const onRotatedSuccess = vi.fn();
    const { container } = render(<LoginView onLoginSuccess={onRotatedSuccess} />);
    fireEvent.change(screen.getByPlaceholderText('admin'), { target: { value: 'admin' } });
    fireEvent.change(screen.getByPlaceholderText('••••••••••••'), { target: { value: 'OldPass123!' } });
    fireEvent.click(screen.getByRole('button', { name: /Sign In/i }));

    await waitFor(() => expect(screen.getByText('Password Expired')).toBeInTheDocument());
    const pwInputs = container.querySelectorAll('input[type="password"]');
    fireEvent.change(pwInputs[0], { target: { value: 'NewStrongPass99!' } });
    fireEvent.change(pwInputs[1], { target: { value: 'NewStrongPass99!' } });
    fireEvent.click(screen.getByRole('button', { name: /Update Password & Sign In/i }));

    await waitFor(() => expect(onRotatedSuccess).toHaveBeenCalledWith(mockAdminUser));
  });
});
