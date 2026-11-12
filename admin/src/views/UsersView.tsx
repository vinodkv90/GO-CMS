import React from 'react';
import { Shield } from 'lucide-react';
import type { User } from '../types';
import { hasPermission } from '../utils/rbac';
import { AdminUsersSettingsView } from './settings/AdminUsersSettingsView';
import { Alert } from '../components/ui/Alert';
import { Button } from '../components/ui/Button';

export interface UsersViewProps {
  currentUser: User;
  onUserUpdated?: (user: User) => void;
  onOpenRoles?: () => void;
}

export const UsersView: React.FC<UsersViewProps> = ({
  currentUser,
  onUserUpdated,
  onOpenRoles,
}) => {
  const canReadUsers = hasPermission(currentUser, 'read', 'users');

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 tracking-tight">Users & Access Management</h1>
          <p className="text-xs text-slate-500 mt-1">
            Invite team members, assign RBAC roles, and manage administrator lifecycle status.
          </p>
        </div>
        {onOpenRoles && hasPermission(currentUser, 'read', 'roles') && (
          <Button type="button" variant="outline" size="sm" onClick={onOpenRoles}>
            <Shield className="w-3.5 h-3.5" />
            <span>Roles & Permissions Matrix</span>
          </Button>
        )}
      </div>

      {canReadUsers ? (
        <AdminUsersSettingsView
          currentUser={currentUser}
          onUserUpdated={onUserUpdated}
          onNavigateRoles={hasPermission(currentUser, 'read', 'roles') ? onOpenRoles : undefined}
        />
      ) : (
        <Alert variant="error" title="Permission required">
          You do not have permission to view administrator accounts.
        </Alert>
      )}
    </div>
  );
};
