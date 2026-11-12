import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ToastProvider } from '../../components/ui/Toast';
import { WebhooksSettingsView } from './WebhooksSettingsView';
import { PluginSettingsView } from './PluginSettingsView';
import { InternationalizationSettingsView } from './InternationalizationSettingsView';
import * as endpoints from '../../api/endpoints';

vi.mock('../../api/endpoints', async () => {
  const actual = await vi.importActual<typeof import('../../api/endpoints')>('../../api/endpoints');
  return {
    ...actual,
    webhooksApi: {
      ...actual.webhooksApi,
      list: vi.fn(),
      listEvents: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      remove: vi.fn(),
      triggerTest: vi.fn(),
      listDeliveries: vi.fn(),
    },
    pluginsApi: {
      ...actual.pluginsApi,
      list: vi.fn(),
      enable: vi.fn(),
      disable: vi.fn(),
      updateSettings: vi.fn(),
    },
    localesApi: {
      ...actual.localesApi,
      list: vi.fn(),
      create: vi.fn(),
      update: vi.fn(),
      setDefault: vi.fn(),
      remove: vi.fn(),
    },
  };
});

describe('M6 Webhooks, Plugins & Internationalization Views', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders WebhooksSettingsView, triggers a test delivery, and displays response snippet', async () => {
    vi.mocked(endpoints.webhooksApi.listEvents).mockResolvedValue({
      data: [
        { name: 'entry.publish', label: 'Entry Published', category: 'Content', description: 'Entry published' },
      ],
    });
    vi.mocked(endpoints.webhooksApi.list).mockResolvedValue({
      data: [
        {
          id: 'wh-1',
          name: 'Deploy Hook',
          url: 'https://hooks.example.com/build',
          headers: {},
          events: ['entry.publish'],
          secret_configured: true,
          is_enabled: true,
          max_retries: 3,
          created_at: '2026-11-12T00:00:00Z',
          updated_at: '2026-11-12T00:00:00Z',
        },
      ],
    });
    vi.mocked(endpoints.webhooksApi.triggerTest).mockResolvedValue({
      data: {
        id: 'del-1',
        webhook_id: 'wh-1',
        event: 'webhook.test',
        request_url: 'https://hooks.example.com/build',
        status_code: 202,
        duration_ms: 14,
        attempt: 1,
        success: true,
        response_snippet: '{"queued":true}',
        created_at: '2026-11-12T00:00:00Z',
      },
    });
    vi.mocked(endpoints.webhooksApi.listDeliveries).mockResolvedValue({
      data: [
        {
          id: 'del-1',
          webhook_id: 'wh-1',
          event: 'webhook.test',
          request_url: 'https://hooks.example.com/build',
          status_code: 202,
          duration_ms: 14,
          attempt: 1,
          success: true,
          response_snippet: '{"queued":true}',
          created_at: '2026-11-12T00:00:00Z',
        },
      ],
      meta: { total: 1, limit: 25, offset: 0 },
    });

    render(
      <ToastProvider>
        <WebhooksSettingsView canCreate canEdit canDelete />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('Deploy Hook')).toBeInTheDocument());
    expect(screen.getByText('HMAC Signed')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /Trigger Test/i }));
    await waitFor(() => expect(screen.getByText('Delivery History: Deploy Hook')).toBeInTheDocument());
    expect(screen.getByText('HTTP 202')).toBeInTheDocument();
    expect(screen.getByText('{"queued":true}')).toBeInTheDocument();
  });

  it('renders PluginSettingsView, toggles plugin state, and edits settingsSchema form', async () => {
    vi.mocked(endpoints.pluginsApi.list).mockResolvedValue({
      data: [
        {
          manifest: {
            id: 'seo-analyzer',
            name: 'SEO Analyzer Plugin',
            version: '1.0.0',
            description: 'Analyzes SEO score',
            author: 'Go CMS Core Team',
            runtime: 'go-native',
            settingsSchema: {
              min_word_count: {
                type: 'number',
                label: 'Minimum Content Word Count',
                default: 50,
                min: 10,
                max: 5000,
              },
            },
            routes: [{ method: 'POST', path: '/analyze', permission: 'read:plugins' }],
          },
          status: 'enabled',
          settings: { min_word_count: 50 },
        },
      ],
    });
    vi.mocked(endpoints.pluginsApi.updateSettings).mockResolvedValue({
      message: 'Settings updated successfully',
      data: {
        manifest: {
          id: 'seo-analyzer',
          name: 'SEO Analyzer Plugin',
          version: '1.0.0',
          description: 'Analyzes SEO score',
          author: 'Go CMS Core Team',
          runtime: 'go-native',
        },
        status: 'enabled',
        settings: { min_word_count: 100 },
      },
    });

    render(
      <ToastProvider>
        <PluginSettingsView canEdit />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('SEO Analyzer Plugin')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Configure/i }));

    await waitFor(() => expect(screen.getByText('Plugin Settings: SEO Analyzer Plugin')).toBeInTheDocument());
    const input = screen.getByDisplayValue('50');
    fireEvent.change(input, { target: { value: '100' } });
    fireEvent.click(screen.getByRole('button', { name: /Save Plugin Settings/i }));

    await waitFor(() =>
      expect(endpoints.pluginsApi.updateSettings).toHaveBeenCalledWith('seo-analyzer', { min_word_count: 100 }),
    );
  });

  it('renders InternationalizationSettingsView, creates a locale, and disables deletion on default locale', async () => {
    vi.mocked(endpoints.localesApi.list).mockResolvedValue({
      data: [
        {
          id: 'loc-en',
          code: 'en',
          name: 'English',
          native_name: 'English',
          is_default: true,
          is_enabled: true,
          direction: 'ltr',
          created_at: '2026-11-12T00:00:00Z',
          updated_at: '2026-11-12T00:00:00Z',
        },
        {
          id: 'loc-ar',
          code: 'ar',
          name: 'Arabic',
          native_name: 'العربية',
          is_default: false,
          is_enabled: true,
          fallback_code: 'en',
          direction: 'rtl',
          created_at: '2026-11-12T00:00:00Z',
          updated_at: '2026-11-12T00:00:00Z',
        },
      ],
    });
    vi.mocked(endpoints.localesApi.setDefault).mockResolvedValue({ message: 'default locale updated successfully' });

    render(
      <ToastProvider>
        <InternationalizationSettingsView canCreate canEdit canDelete />
      </ToastProvider>,
    );

    await waitFor(() => expect(screen.getByText('English')).toBeInTheDocument());
    expect(screen.getByTitle('Cannot delete the default locale')).toBeDisabled();

    fireEvent.click(screen.getByRole('button', { name: /Set Default/i }));
    await waitFor(() => expect(endpoints.localesApi.setDefault).toHaveBeenCalledWith('ar'));
  });
});

