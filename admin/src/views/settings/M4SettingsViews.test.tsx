import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ToastProvider } from '../../components/ui/Toast';
import { MediaSettingsView } from './MediaSettingsView';
import { StorageProvidersView } from './StorageProvidersView';
import { EmailSettingsView } from './EmailSettingsView';
import { EmailTemplatesView } from './EmailTemplatesView';
import * as endpoints from '../../api/endpoints';

vi.mock('../../api/endpoints', async () => {
  const actual = await vi.importActual<typeof import('../../api/endpoints')>('../../api/endpoints');
  return {
    ...actual,
    mediaFoldersApi: {
      ...actual.mediaFoldersApi,
      list: vi.fn(),
    },
    settingsApi: {
      ...actual.settingsApi,
      getMedia: vi.fn(),
      updateMedia: vi.fn(),
      getStorage: vi.fn(),
      updateStorage: vi.fn(),
      testStorage: vi.fn(),
    },
    emailApi: {
      ...actual.emailApi,
      getSettings: vi.fn(),
      updateSettings: vi.fn(),
      testConnection: vi.fn(),
      testSend: vi.fn(),
    },
    emailTemplatesApi: {
      ...actual.emailTemplatesApi,
      list: vi.fn(),
      get: vi.fn(),
      update: vi.fn(),
      preview: vi.fn(),
    },
  };
});

describe('M4 Settings Views', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders MediaSettingsView and adds MIME types', async () => {
    vi.mocked(endpoints.mediaFoldersApi.list).mockResolvedValue({
      data: [
        {
          id: 'fld-1',
          name: 'Avatars',
          slug: 'avatars',
          path: 'avatars',
          storage_path: 'avatars/',
          created_at: '2026-11-12T00:00:00Z',
          updated_at: '2026-11-12T00:00:00Z',
        },
      ],
    });
    vi.mocked(endpoints.settingsApi.getMedia).mockResolvedValue({
      data: {
        max_file_size_bytes: 10 * 1024 * 1024,
        allowed_mime_types: ['image/*', 'application/pdf'],
        responsive_sizes_enabled: true,
        generate_small: true,
        generate_medium: true,
        generate_large: false,
        require_alt_text: false,
        default_folder_id: null,
        default_access: 'public',
        signed_url_ttl_seconds: 3600,
      },
    });

    const onDirtyChange = vi.fn();
    render(
      <ToastProvider>
        <MediaSettingsView canEdit onDirtyChange={onDirtyChange} registerHandlers={vi.fn()} />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('Upload Limits & File Validation')).toBeInTheDocument());
    expect(screen.getByText('image/*')).toBeInTheDocument();

    const input = screen.getByPlaceholderText('e.g. image/* or application/pdf');
    fireEvent.change(input, { target: { value: 'video/mp4' } });
    fireEvent.click(screen.getByRole('button', { name: /Add MIME/i }));
    expect(screen.getByText('video/mp4')).toBeInTheDocument();
  });

  it('displays honest unavailable badges for GCS/Azure and runs Test Connection on StorageProvidersView', async () => {
    vi.mocked(endpoints.settingsApi.getStorage).mockResolvedValue({
      data: {
        active_provider: 'local',
        local: { upload_dir: './uploads', url_prefix: '/uploads' },
        s3: {
          bucket: '',
          region: 'us-east-1',
          endpoint: '',
          access_key_id: '',
          secret_access_key: '',
          secret_configured: false,
          use_ssl: true,
          force_path_style: false,
          base_url: '',
        },
        supported_providers: [
          { id: 'local', label: 'Local Filesystem', implemented: true },
          { id: 's3', label: 'Amazon S3', implemented: true },
          { id: 'minio', label: 'MinIO / S3-Compatible', implemented: true },
          {
            id: 'gcs',
            label: 'Google Cloud Storage',
            implemented: false,
            reason: 'Google Cloud Storage driver is not included in this build.',
          },
          {
            id: 'azure',
            label: 'Azure Blob Storage',
            implemented: false,
            reason: 'Azure Blob Storage driver is not included in this build.',
          },
        ],
      },
    });
    vi.mocked(endpoints.settingsApi.testStorage).mockResolvedValue({
      data: {
        ok: true,
        provider: 'local',
        latency_ms: 2,
        message: 'Storage provider "local" is healthy and reachable.',
      },
    });

    render(
      <ToastProvider>
        <StorageProvidersView canEdit onDirtyChange={vi.fn()} registerHandlers={vi.fn()} />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('Active Storage Driver')).toBeInTheDocument());
    expect(screen.getByText('Google Cloud Storage').closest('button')).toBeDisabled();
    expect(screen.getByText('Azure Blob Storage').closest('button')).toBeDisabled();

    fireEvent.click(screen.getByRole('button', { name: /Test Connection/i }));
    await waitFor(() =>
      expect(screen.getByText(/Storage provider "local" is healthy and reachable\./i)).toBeInTheDocument(),
    );
  });

  it('tests email connection and opens Send Test Email dialog in EmailSettingsView', async () => {
    vi.mocked(endpoints.emailApi.getSettings).mockResolvedValue({
      data: {
        provider: 'log',
        from_name: 'Go CMS',
        from_email: 'noreply@example.org',
        default_reply_to: '',
        smtp_host: '',
        smtp_port: 587,
        smtp_username: '',
        smtp_password: '',
        password_configured: false,
        encryption: 'starttls',
        skip_tls_verify: false,
        timeout_seconds: 10,
      },
    });
    vi.mocked(endpoints.emailApi.testConnection).mockResolvedValue({
      data: { ok: true, provider: 'log', latency_ms: 1, message: 'Email provider "log" is ready.' },
    });
    vi.mocked(endpoints.emailApi.testSend).mockResolvedValue({
      data: { ok: true, provider: 'log', latency_ms: 3, message: 'Test email dispatched via log to qa@example.org.' },
    });

    render(
      <ToastProvider>
        <EmailSettingsView canEdit onDirtyChange={vi.fn()} registerHandlers={vi.fn()} />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('Outbound Mail Transport')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Test Connection/i }));
    await waitFor(() => expect(screen.getByText(/Email provider "log" is ready\./i)).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: /Send Test Email/i }));
    const emailInput = screen.getByPlaceholderText('you@example.com');
    fireEvent.change(emailInput, { target: { value: 'qa@example.org' } });
    fireEvent.click(screen.getByRole('button', { name: /^Send Test$/i }));
    await waitFor(() => expect(endpoints.emailApi.testSend).toHaveBeenCalledWith('qa@example.org', expect.anything()));
  });

  it('renders EmailTemplatesView list and previews sanitized template HTML', async () => {
    vi.mocked(endpoints.emailTemplatesApi.list).mockResolvedValue({
      data: [
        {
          id: 'tpl-1',
          template_key: 'admin_invite',
          name: 'Admin Panel Invitation',
          subject: 'Invite to {{site_name}}',
          from_name: '',
          from_email: '',
          reply_to: '',
          body_html: '<p>Join {{invite_url}}</p>',
          body_text: 'Join {{invite_url}}',
          variables: ['site_name', 'invite_url'],
          enabled: true,
          updated_at: '2026-11-12T00:00:00Z',
        },
      ],
    });
    vi.mocked(endpoints.emailTemplatesApi.preview).mockResolvedValue({
      data: {
        subject: 'Invite to Go CMS',
        body_html: '<p>Join https://cms.example.com/admin/?invite_token=sample</p>',
        body_text: 'Join https://cms.example.com/admin/?invite_token=sample',
      },
    });

    render(
      <ToastProvider>
        <EmailTemplatesView canEdit />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('Transactional Templates')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Preview/i }));
    await waitFor(() => expect(screen.getByText('Invite to Go CMS')).toBeInTheDocument());
  });

  it('selects Cloudinary provider, validates, tests connection, and saves settings', async () => {
    vi.mocked(endpoints.settingsApi.getStorage).mockResolvedValue({
      data: {
        active_provider: 'local',
        local: { upload_dir: './uploads', url_prefix: '/uploads' },
        s3: {
          bucket: '',
          region: 'us-east-1',
          endpoint: '',
          access_key_id: '',
          secret_access_key: '',
          secret_configured: false,
          use_ssl: true,
          force_path_style: false,
          base_url: '',
        },
        cloudinary: {
          cloud_name: '',
          api_key: '',
          api_secret: '',
          secret_configured: false,
          root_folder: '',
          delivery_type: 'upload',
          default_resource_type: 'auto',
          use_secure_url: true,
          upload_preset: '',
          asset_naming_strategy: 'cms_key',
          optimization_enabled: true,
          default_format: 'auto',
          default_quality: 'auto',
        },
        supported_providers: [
          { id: 'local', label: 'Local Filesystem', implemented: true },
          { id: 's3', label: 'Amazon S3', implemented: true },
          { id: 'minio', label: 'MinIO / S3-Compatible', implemented: true },
          { id: 'cloudinary', label: 'Cloudinary', implemented: true },
          { id: 'gcs', label: 'Google Cloud Storage', implemented: false, reason: 'Unavailable' },
          { id: 'azure', label: 'Azure Blob Storage', implemented: false, reason: 'Unavailable' },
        ],
      },
    });
    vi.mocked(endpoints.settingsApi.testStorage).mockResolvedValue({
      data: {
        ok: true,
        provider: 'cloudinary',
        latency_ms: 42,
        message: 'Storage provider "cloudinary" is healthy and reachable.',
      },
    });
    vi.mocked(endpoints.settingsApi.updateStorage).mockImplementation(async (payload) => ({
      data: {
        ...payload,
        cloudinary: {
          ...(payload.cloudinary || {
            cloud_name: 'demo-cloud',
            api_key: '123456',
            root_folder: 'go-cms',
            delivery_type: 'upload',
            default_resource_type: 'auto',
            use_secure_url: true,
            optimization_enabled: true,
          }),
          api_secret: '__PRESERVE__',
          secret_configured: true,
        },
      },
    }));

    let registeredHandlers: { save: () => Promise<void>; discard: () => void; isSaving: boolean } | null = null;
    render(
      <ToastProvider>
        <StorageProvidersView
          canEdit
          onDirtyChange={vi.fn()}
          registerHandlers={(h) => {
            registeredHandlers = h;
          }}
        />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('Cloudinary')).toBeInTheDocument());
    fireEvent.click(screen.getByText('Cloudinary').closest('button')!);
    expect(screen.getByText('Cloudinary Configuration')).toBeInTheDocument();

    // Testing connection before entering credentials shows client-side validation error
    fireEvent.click(screen.getByRole('button', { name: /Test Connection/i }));
    await waitFor(() => expect(screen.getByText(/Cloudinary Cloud Name is required/i)).toBeInTheDocument());

    // Enter valid Cloudinary credentials
    fireEvent.change(screen.getByPlaceholderText('your-cloud-name'), {
      target: { value: 'demo-cloud' },
    });
    fireEvent.change(screen.getByPlaceholderText('Cloudinary API Key'), {
      target: { value: '123456' },
    });
    fireEvent.change(screen.getByPlaceholderText('Enter secret value'), {
      target: { value: 'my-secret-token' },
    });
    fireEvent.change(screen.getByPlaceholderText('go-cms'), {
      target: { value: 'cms-prod' },
    });

    fireEvent.click(screen.getByRole('button', { name: /Test Connection/i }));
    await waitFor(() =>
      expect(screen.getByText(/Storage provider "cloudinary" is healthy and reachable/i)).toBeInTheDocument(),
    );

    await registeredHandlers!.save();
    expect(endpoints.settingsApi.updateStorage).toHaveBeenCalledWith(
      expect.objectContaining({
        active_provider: 'cloudinary',
        cloudinary: expect.objectContaining({
          cloud_name: 'demo-cloud',
          api_key: '123456',
          api_secret: 'my-secret-token',
          root_folder: 'cms-prod',
        }),
      }),
    );
  });
});
