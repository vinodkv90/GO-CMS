import React, { useState, useEffect } from 'react';
import { Shield, Mail, UserCheck } from 'lucide-react';
import { Card, CardHeader } from '../components/ui/Card';
import { usersApi } from '../api/endpoints';
import type { User } from '../types';

export const UsersView: React.FC = () => {
  const [users, setUsers] = useState<User[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchUsers = async () => {
      try {
        const res = await usersApi.list();
        setUsers(res.data || []);
      } catch (err) {
        console.error('Failed to fetch users:', err);
      } finally {
        setIsLoading(false);
      }
    };
    fetchUsers();
  }, []);

  return (
    <div className="space-y-6">
      {/* Top Header */}
      <div>
        <h1 className="text-2xl font-bold text-slate-900 tracking-tight">Users & Access Management</h1>
        <p className="text-xs text-slate-500 mt-1">
          Manage system administrators, editors, role assignments (RBAC), and API tokens.
        </p>
      </div>

      {/* Users Table */}
      <Card>
        <CardHeader
          title="Administrative & Member Accounts"
          description="Users authenticated via Argon2id password security and scoped RBAC permissions"
        />
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse text-xs">
            <thead>
              <tr className="bg-slate-50/75 border-b border-slate-100 text-slate-500 font-semibold uppercase tracking-wider text-[10px]">
                <th className="px-6 py-3">User</th>
                <th className="px-6 py-3">Email Address</th>
                <th className="px-6 py-3">Status</th>
                <th className="px-6 py-3">Assigned Roles</th>
                <th className="px-6 py-3 text-right">Access Level</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 text-slate-700">
              {isLoading ? (
                <tr>
                  <td colSpan={5} className="px-6 py-8 text-center text-slate-400">
                    Loading accounts...
                  </td>
                </tr>
              ) : users.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-6 py-8 text-center text-slate-400">
                    No users found.
                  </td>
                </tr>
              ) : (
                users.map((user) => (
                  <tr key={user.id} className="hover:bg-slate-50/50">
                    <td className="px-6 py-3.5">
                      <div className="flex items-center gap-3">
                        <div className="w-8 h-8 rounded-full bg-slate-100 flex items-center justify-center font-bold text-slate-600 text-xs">
                          {user.username.substring(0, 2).toUpperCase()}
                        </div>
                        <div>
                          <p className="font-bold text-slate-900">{user.username}</p>
                          <p className="text-[10px] text-slate-400 font-mono">{user.id}</p>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-3.5 text-slate-600">
                      <div className="flex items-center gap-1.5">
                        <Mail className="w-3.5 h-3.5 text-slate-400" />
                        <span>{user.email}</span>
                      </div>
                    </td>
                    <td className="px-6 py-3.5">
                      <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-emerald-50 text-emerald-700 border border-emerald-100 capitalize">
                        {user.status || 'active'}
                      </span>
                    </td>
                    <td className="px-6 py-3.5 space-x-1.5">
                      {user.roles && user.roles.length > 0 ? (
                        user.roles.map((r) => (
                          <span
                            key={r.slug || r.name}
                            className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-semibold bg-indigo-50 text-indigo-700 border border-indigo-100"
                          >
                            <Shield className="w-3 h-3" />
                            <span>{r.name || r.slug}</span>
                          </span>
                        ))
                      ) : (
                        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-semibold bg-slate-100 text-slate-600">
                          <UserCheck className="w-3 h-3" />
                          <span>Administrator</span>
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-3.5 text-right font-mono text-[11px] text-slate-500">
                      Full Read / Write
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
};
