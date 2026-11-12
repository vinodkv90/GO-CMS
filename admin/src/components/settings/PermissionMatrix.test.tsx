import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { PermissionMatrix } from './PermissionMatrix';
import type { Permission } from '../../types';

const SAMPLE_PERMS: Permission[] = [
  { id: 'perm_all', action: '*', resource: '*', description: 'Full access' },
  { id: 'perm_content_read', action: 'read', resource: 'content', description: 'Read content' },
  { id: 'perm_content_create', action: 'create', resource: 'content', description: 'Create content' },
  { id: 'perm_media_read', action: 'read', resource: 'media', description: 'Read media' },
  { id: 'perm_users_read', action: 'read', resource: 'users', description: 'Read users' },
];

describe('PermissionMatrix', () => {
  it('toggles individual permissions and resource select-all', () => {
    const onChange = vi.fn();
    render(
      <PermissionMatrix
        permissions={SAMPLE_PERMS}
        selectedIds={['perm_content_read']}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByLabelText('Create'));
    expect(onChange).toHaveBeenCalledWith(['perm_content_read', 'perm_content_create']);

    const allButtons = screen.getAllByText('All');
    fireEvent.click(allButtons[0]);
    expect(onChange).toHaveBeenCalledWith(['perm_content_read', 'perm_content_create']);
  });

  it('locks controls when isSuperAdminLocked is true', () => {
    const onChange = vi.fn();
    render(
      <PermissionMatrix
        permissions={SAMPLE_PERMS}
        selectedIds={['perm_all']}
        onChange={onChange}
        isSuperAdminLocked
      />,
    );

    expect(screen.getByText(/Super Administrator/i)).toBeInTheDocument();
    const createCheckbox = screen.getByLabelText('Create') as HTMLInputElement;
    expect(createCheckbox.disabled).toBe(true);
    fireEvent.click(createCheckbox);
    expect(onChange).not.toHaveBeenCalled();
  });
});

