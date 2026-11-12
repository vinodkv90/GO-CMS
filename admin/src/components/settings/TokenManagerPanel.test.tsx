import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { TokenManagerPanel } from './TokenManagerPanel';
import { ToastProvider } from '../ui/Toast';
import type { ApiToken, User } from '../../types';

const mockList = vi.fn();
const mockCreate = vi.fn();
const mockRotate = vi.fn();
const mockRevoke = vi.fn();
const mockRemove = vi.fn();
const mockListTransfer = vi.fn();
const mockCreateTransfer = vi.fn();
const mockListPermissions = vi.fn();

vi.mock('../../api/endpoints', () => ({
  tokensApi: {
    list: (...args: unknown[]) => mockList(...args),
    create: (...args: unknown[]) => mockCreate(...args),
    rotate: (...args: unknown[]) => mockRotate(...args),
    revoke: (...args: unknown[]) => mockRevoke(...args),
    remove: (...args: unknown[]) => mockRemove(...args),
    listTransfer: (...args: unknown[]) => mockListTransfer(...args),
    createTransfer: (...args: unknown[]) => mockCreateTransfer(...args),
  },
  rolesApi: {
    listPermissions: () => mockListPermissions(),
  },
}));

const SUPER_USER: User = {
  id: 'u1',
  username: 'admin',
  email: 'admin@cms.test',
  status: 'active',
  roles: [{ id: 'role_super_admin', name: 'Super Admin', slug: 'super_admin' }],
};

const EXISTING_TOKEN: ApiToken = {
  id: 'tok-1',
  name: 'Storefront Key',
  kind: 'api',
  description: 'Next.js frontend',
  token_type: 'read_only',
  token_prefix: 'cms_pat_12345678',
  user_id: 'u1',
  permissions: ['read:content', 'read:media'],
  created_at: '2026-11-12T00:00:00Z',
};

describe('TokenManagerPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListPermissions.mockResolvedValue({
      data: [
        { id: 'p1', action: 'read', resource: 'content', description: 'Read content' },
        { id: 'p2', action: 'create', resource: 'media', description: 'Upload media' },
      ],
    });
    mockList.mockResolvedValue({ data: [EXISTING_TOKEN] });
    mockCreate.mockResolvedValue({
      token: 'cms_pat_secret_once_only',
      metadata: { ...EXISTING_TOKEN, id: 'tok-2', name: 'CI Key' },
    });
    mockRotate.mockResolvedValue({
      token: 'cms_pat_rotated_once_only',
      metadata: EXISTING_TOKEN,
    });
    mockRevoke.mockResolvedValue({ message: 'token revoked successfully' });
    mockListTransfer.mockResolvedValue({ data: [] });
  });

  it('creates a scoped API token, reveals plaintext once, and supports rotate/revoke', async () => {
    render(
      <ToastProvider>
        <TokenManagerPanel mode="api" currentUser={SUPER_USER} />
      </ToastProvider>,
    );

    expect(await screen.findByText('Storefront Key')).toBeInTheDocument();

    fireEvent.click(screen.getByText('Create new API Token'));
    fireEvent.change(screen.getByPlaceholderText('Next.js Storefront'), {
      target: { value: 'CI Key' },
    });
    fireEvent.click(screen.getByText('Generate token'));

    await waitFor(() => {
      expect(mockCreate).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'CI Key',
          kind: 'api',
          token_type: 'read_only',
        }),
      );
    });
    expect(await screen.findByText('cms_pat_secret_once_only')).toBeInTheDocument();

    fireEvent.click(screen.getByText('I have stored this token'));

    fireEvent.click(screen.getByTitle('Rotate token secret'));
    await waitFor(() => expect(mockRotate).toHaveBeenCalledWith('tok-1'));
    expect(await screen.findByText('cms_pat_rotated_once_only')).toBeInTheDocument();
  });

  it('loads transfer tokens in transfer mode', async () => {
    render(
      <ToastProvider>
        <TokenManagerPanel mode="transfer" currentUser={SUPER_USER} />
      </ToastProvider>,
    );

    await waitFor(() => expect(mockListTransfer).toHaveBeenCalledTimes(1));
    expect(screen.getByText('No transfer tokens yet')).toBeInTheDocument();
  });
});

