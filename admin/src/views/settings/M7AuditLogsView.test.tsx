import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { ToastProvider } from '../../components/ui/Toast';
import { AuditLogsSettingsView } from './AuditLogsSettingsView';
import { auditApi } from '../../api/endpoints';

vi.mock('../../api/endpoints', () => ({
  auditApi: {
    list: vi.fn(),
    exportCsv: vi.fn(),
  },
}));

const renderView = () =>
  render(
    <ToastProvider>
      <AuditLogsSettingsView />
    </ToastProvider>,
  );

describe('M7 Audit Logs Settings View', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(auditApi.list).mockResolvedValue({
      data: [
        {
          id: 'log-1',
          actor_id: 'admin-uuid-1',
          actor_type: 'admin',
          action: 'settings.update',
          resource: 'settings:email',
          target_id: 'email',
          result: 'success',
          ip_address: '127.0.0.1',
          user_agent: 'VitestBrowser/1.0',
          metadata: { smtp_host: 'smtp.example.com', smtp_password: '[REDACTED]' },
          created_at: '2026-11-12T10:00:00Z',
        },
        {
          id: 'log-2',
          actor_id: 'admin-uuid-1',
          actor_type: 'admin',
          action: 'admin_login_failed',
          resource: 'auth:session',
          target_id: 'admin-uuid-1',
          result: 'failure',
          ip_address: '127.0.0.1',
          metadata: { reason: 'invalid_credentials' },
          created_at: '2026-11-12T09:55:00Z',
        },
      ],
      meta: { total: 2, limit: 25, offset: 0 },
    });
    vi.mocked(auditApi.exportCsv).mockResolvedValue(
      new Blob(['id,action\nlog-1,settings.update\n'], { type: 'text/csv' }),
    );
  });

  it('loads audit logs, filters by action/result, and opens redacted metadata modal', async () => {
    renderView();

    expect(await screen.findByText('settings.update')).toBeInTheDocument();
    expect(screen.getByText('admin_login_failed')).toBeInTheDocument();

    // Open Inspect modal for redacted secret verification
    fireEvent.click(screen.getByLabelText('Inspect audit entry settings.update'));
    expect(await screen.findByText('Audit Event Details')).toBeInTheDocument();
    expect(screen.getByText(/"smtp_password": "\[REDACTED\]"/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));

    // Apply action & result filters
    fireEvent.change(screen.getByLabelText('Action'), { target: { value: 'admin_login_failed' } });
    fireEvent.change(screen.getByLabelText('Result'), { target: { value: 'failure' } });
    fireEvent.click(screen.getByRole('button', { name: 'Apply Filters' }));

    await waitFor(() => {
      expect(auditApi.list).toHaveBeenCalledWith(
        expect.objectContaining({
          action: 'admin_login_failed',
          result: 'failure',
          limit: 25,
          offset: 0,
        }),
      );
    });
  });

  it('triggers CSV export download using active filters', async () => {
    const origCreateObjectURL = URL.createObjectURL;
    const origRevokeObjectURL = URL.revokeObjectURL;
    URL.createObjectURL = vi.fn(() => 'blob:mock-audit-csv');
    URL.revokeObjectURL = vi.fn();

    try {
      renderView();
      expect(await screen.findByText('settings.update')).toBeInTheDocument();

      fireEvent.click(screen.getByRole('button', { name: 'Export CSV' }));

      await waitFor(() => {
        expect(auditApi.exportCsv).toHaveBeenCalledTimes(1);
        expect(URL.createObjectURL).toHaveBeenCalledTimes(1);
      });
    } finally {
      URL.createObjectURL = origCreateObjectURL;
      URL.revokeObjectURL = origRevokeObjectURL;
    }
  });
});
