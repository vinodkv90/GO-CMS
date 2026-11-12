import React, { useEffect, useRef, useState } from 'react';
import {
  ChevronDown,
  ExternalLink,
  LogOut,
  Settings,
  ShieldCheck,
  User as UserIcon,
} from 'lucide-react';
import { hasAnyPermission } from '../../utils/rbac';
import type { User } from '../../types';

export interface TopbarProps {
  user: User | null;
  pageTitle: string;
  onSelectTab?: (tab: string, param?: string) => void;
  onLogout?: () => void;
}

export const Topbar: React.FC<TopbarProps> = ({
  user,
  pageTitle,
  onSelectTab,
  onLogout,
}) => {
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const roleName = user?.roles?.[0]?.name || user?.roles?.[0]?.slug || 'Administrator';
  const canOpenSettings = hasAnyPermission(user, [
    ['read', 'settings'],
    ['read', 'system:info'],
  ]);

  useEffect(() => {
    if (!menuOpen) return;
    const handlePointerDown = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    };
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMenuOpen(false);
    };
    document.addEventListener('mousedown', handlePointerDown);
    window.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [menuOpen]);

  return (
    <header className="h-16 bg-white border-b border-slate-200/80 px-8 flex items-center justify-between shrink-0 select-none">
      <div className="flex items-center gap-3">
        <h2 className="text-lg font-bold text-slate-800 tracking-tight">{pageTitle}</h2>
        <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-sky-50 text-sky-700 border border-sky-100 uppercase tracking-wider">
          v1.0.0
        </span>
      </div>

      <div className="flex items-center gap-4">
        {/* View Public Website */}
        <a
          href="/"
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-slate-200 text-xs font-semibold text-slate-600 hover:text-sky-600 hover:border-sky-200 transition-colors"
        >
          <span>View Site</span>
          <ExternalLink className="w-3.5 h-3.5" />
        </a>

        <div className="h-4 w-px bg-slate-200" />

        {/* Interactive User Account Menu */}
        <div className="relative" ref={menuRef}>
          <button
            type="button"
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen((prev) => !prev)}
            className="flex items-center gap-2.5 rounded-lg px-2 py-1 hover:bg-slate-50 transition-colors cursor-pointer"
          >
            <div className="w-8 h-8 rounded-full bg-slate-100 border border-slate-200 flex items-center justify-center text-slate-600">
              <UserIcon className="w-4 h-4" />
            </div>
            <div className="text-left">
              <p className="text-xs font-bold text-slate-800 leading-none">
                {user?.first_name ? `${user.first_name} ${user.last_name || ''}`.trim() : user?.username || 'Admin'}
              </p>
              <div className="flex items-center gap-1 mt-1 text-[10px] text-slate-500 font-medium">
                <ShieldCheck className="w-3 h-3 text-emerald-600" />
                <span className="capitalize">{roleName}</span>
              </div>
            </div>
            <ChevronDown className="w-3.5 h-3.5 text-slate-400" />
          </button>

          {menuOpen && (
            <div
              role="menu"
              className="absolute right-0 mt-2 w-56 rounded-xl bg-white border border-slate-200 shadow-lg py-1.5 z-50 text-xs"
            >
              <div className="px-3.5 py-2 border-b border-slate-100">
                <p className="font-semibold text-slate-900 truncate">{user?.username}</p>
                <p className="text-[11px] text-slate-500 truncate">{user?.email}</p>
              </div>
              {onSelectTab && (
                <button
                  type="button"
                  role="menuitem"
                  onClick={() => {
                    setMenuOpen(false);
                    onSelectTab('settings', 'profile');
                  }}
                  className="w-full flex items-center gap-2.5 px-3.5 py-2 text-slate-700 hover:bg-slate-50 cursor-pointer text-left"
                >
                  <UserIcon className="w-3.5 h-3.5 text-slate-400" />
                  <span>Profile &amp; Preferences</span>
                </button>
              )}
              {onSelectTab && canOpenSettings && (
                <button
                  type="button"
                  role="menuitem"
                  onClick={() => {
                    setMenuOpen(false);
                    onSelectTab('settings', 'overview');
                  }}
                  className="w-full flex items-center gap-2.5 px-3.5 py-2 text-slate-700 hover:bg-slate-50 cursor-pointer text-left"
                >
                  <Settings className="w-3.5 h-3.5 text-slate-400" />
                  <span>Platform Settings</span>
                </button>
              )}
              {onLogout && (
                <div className="border-t border-slate-100 mt-1 pt-1">
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setMenuOpen(false);
                      onLogout();
                    }}
                    className="w-full flex items-center gap-2.5 px-3.5 py-2 text-rose-600 hover:bg-rose-50 cursor-pointer text-left font-medium"
                  >
                    <LogOut className="w-3.5 h-3.5" />
                    <span>Sign Out</span>
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </header>
  );
};

