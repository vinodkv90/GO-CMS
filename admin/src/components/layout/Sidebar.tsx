import React from 'react';
import {
  LayoutDashboard,
  Layers,
  FileText,
  Image,
  Puzzle,
  Palette,
  Users,
  Settings,
  LogOut,
} from 'lucide-react';
import { hasAnyPermission } from '../../utils/rbac';
import type { ContentType, PublicGeneralSettings, User } from '../../types';

export interface SidebarProps {
  currentTab: string;
  onSelectTab: (tab: string, param?: string) => void;
  contentTypes: ContentType[];
  user?: User | null;
  publicSettings?: PublicGeneralSettings | null;
  collapsed?: boolean;
  onLogout: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({
  currentTab,
  onSelectTab,
  contentTypes,
  user,
  publicSettings,
  collapsed = false,
  onLogout,
}) => {
  const siteName = publicSettings?.site_name || 'Go CMS';
  const initialLetter = siteName.trim().charAt(0).toUpperCase() || 'G';
  const showSettingsNav =
    !user ||
    hasAnyPermission(user, [
      ['read', 'settings'],
      ['read', 'system:info'],
    ]);

  return (
    <aside
      className={`${
        collapsed ? 'w-20' : 'w-64'
      } bg-slate-900 text-slate-300 flex flex-col h-screen select-none shrink-0 border-r border-slate-800 transition-all`}
    >
      {/* Brand Header */}
      <div className="p-5 border-b border-slate-800/80 flex items-center gap-3">
        {publicSettings?.logo_url ? (
          <img
            src={publicSettings.logo_url}
            alt={siteName}
            className="w-9 h-9 rounded-xl bg-white object-contain p-1 shrink-0 shadow-md"
          />
        ) : (
          <div className="w-9 h-9 rounded-xl bg-gradient-to-tr from-sky-500 to-indigo-600 flex items-center justify-center text-white font-black text-lg shadow-md shadow-sky-500/20 shrink-0">
            {initialLetter}
          </div>
        )}
        {!collapsed && (
          <div className="min-w-0">
            <h1 className="font-bold text-white text-base tracking-tight truncate">{siteName}</h1>
            <p className="text-[10px] text-sky-400 font-medium uppercase tracking-wider">Admin Console</p>
          </div>
        )}
      </div>

      {/* Navigation Links */}
      <nav className="flex-1 overflow-y-auto px-3 py-4 space-y-6 text-xs">
        {/* Core System */}
        <div className="space-y-1">
          <p className="px-3 text-[10px] font-bold text-slate-400 uppercase tracking-wider mb-2">Core</p>
          <button
            onClick={() => onSelectTab('dashboard')}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg font-medium transition-colors cursor-pointer ${
              currentTab === 'dashboard'
                ? 'bg-sky-600 text-white shadow-xs'
                : 'text-slate-300 hover:bg-slate-800/60 hover:text-white'
            }`}
          >
            <LayoutDashboard className="w-4 h-4" />
            <span>Dashboard</span>
          </button>
        </div>

        {/* Content Manager by Type */}
        <div className="space-y-1">
          <div className="px-3 flex items-center justify-between mb-2">
            <p className="text-[10px] font-bold text-slate-400 uppercase tracking-wider">Content Manager</p>
          </div>
          {contentTypes.filter((type) => type.kind !== 'component').length === 0 ? (
            <div className="px-3 py-2 text-slate-400 italic text-[11px]">No content types yet</div>
          ) : (
            contentTypes.filter((type) => type.kind !== 'component').map((ct) => (
              <button
                key={ct.uid}
                onClick={() => onSelectTab('content', ct.uid)}
                className={`w-full flex items-center justify-between px-3 py-2 rounded-lg font-medium transition-colors cursor-pointer ${
                  currentTab === `content:${ct.uid}`
                    ? 'bg-sky-600 text-white shadow-xs'
                    : 'text-slate-300 hover:bg-slate-800/60 hover:text-white'
                }`}
              >
                <div className="flex items-center gap-2.5 truncate">
                  <FileText className="w-3.5 h-3.5 shrink-0" />
                  <span className="truncate">{ct.name}</span>
                </div>
                <span className="text-[9px] px-1.5 py-0.5 rounded-full bg-slate-800 text-slate-400 font-mono">
                  {ct.kind === 'single' ? '1' : 'N'}
                </span>
              </button>
            ))
          )}
        </div>

        {/* Studio & Builders */}
        <div className="space-y-1">
          <p className="px-3 text-[10px] font-bold text-slate-400 uppercase tracking-wider mb-2">Studio</p>
          <button
            onClick={() => onSelectTab('content-types')}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg font-medium transition-colors cursor-pointer ${
              currentTab === 'content-types'
                ? 'bg-sky-600 text-white shadow-xs'
                : 'text-slate-300 hover:bg-slate-800/60 hover:text-white'
            }`}
          >
            <Layers className="w-4 h-4" />
            <span>Content Type Builder</span>
          </button>
          <button
            onClick={() => onSelectTab('media')}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg font-medium transition-colors cursor-pointer ${
              currentTab === 'media'
                ? 'bg-sky-600 text-white shadow-xs'
                : 'text-slate-300 hover:bg-slate-800/60 hover:text-white'
            }`}
          >
            <Image className="w-4 h-4" />
            <span>Media Library</span>
          </button>
        </div>

        {/* Extensions & System */}
        <div className="space-y-1">
          <p className="px-3 text-[10px] font-bold text-slate-400 uppercase tracking-wider mb-2">Extensions</p>
          <button
            onClick={() => onSelectTab('plugins')}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg font-medium transition-colors cursor-pointer ${
              currentTab === 'plugins'
                ? 'bg-sky-600 text-white shadow-xs'
                : 'text-slate-300 hover:bg-slate-800/60 hover:text-white'
            }`}
          >
            <Puzzle className="w-4 h-4" />
            <span>Plugins</span>
          </button>
          <button
            onClick={() => onSelectTab('themes')}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg font-medium transition-colors cursor-pointer ${
              currentTab === 'themes'
                ? 'bg-sky-600 text-white shadow-xs'
                : 'text-slate-300 hover:bg-slate-800/60 hover:text-white'
            }`}
          >
            <Palette className="w-4 h-4" />
            <span>Themes</span>
          </button>
          <button
            onClick={() => onSelectTab('users')}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg font-medium transition-colors cursor-pointer ${
              currentTab === 'users'
                ? 'bg-sky-600 text-white shadow-xs'
                : 'text-slate-300 hover:bg-slate-800/60 hover:text-white'
            }`}
          >
            <Users className="w-4 h-4 shrink-0" />
            {!collapsed && <span>Users & Access</span>}
          </button>
          {showSettingsNav && (
            <button
              onClick={() => onSelectTab('settings', 'overview')}
              className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg font-medium transition-colors cursor-pointer ${
                currentTab === 'settings' || currentTab.startsWith('settings:')
                  ? 'bg-sky-600 text-white shadow-xs'
                  : 'text-slate-300 hover:bg-slate-800/60 hover:text-white'
              }`}
            >
              <Settings className="w-4 h-4 shrink-0" />
              {!collapsed && <span>Settings</span>}
            </button>
          )}
        </div>
      </nav>

      {/* Footer Profile & Logout */}
      <div className="p-3 border-t border-slate-800/80">
        <button
          onClick={onLogout}
          className="w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-slate-400 hover:bg-rose-950/40 hover:text-rose-400 transition-colors text-xs font-medium cursor-pointer"
        >
          <LogOut className="w-4 h-4" />
          <span>Sign Out</span>
        </button>
      </div>
    </aside>
  );
};
