import React from 'react';
import { Sidebar } from './Sidebar';
import { Topbar } from './Topbar';
import type { ContentType, PublicGeneralSettings, User } from '../../types';

export interface LayoutProps {
  user: User | null;
  currentTab: string;
  onSelectTab: (tab: string, param?: string) => void;
  contentTypes: ContentType[];
  publicSettings?: PublicGeneralSettings | null;
  sidebarCollapsed?: boolean;
  onLogout: () => void;
  pageTitle: string;
  fullBleed?: boolean;
  children: React.ReactNode;
}

export const Layout: React.FC<LayoutProps> = ({
  user,
  currentTab,
  onSelectTab,
  contentTypes,
  publicSettings,
  sidebarCollapsed = false,
  onLogout,
  pageTitle,
  fullBleed = false,
  children,
}) => {
  return (
    <div className="flex h-screen w-screen overflow-hidden bg-slate-50 font-sans antialiased text-slate-800">
      <Sidebar
        currentTab={currentTab}
        onSelectTab={onSelectTab}
        contentTypes={contentTypes}
        user={user}
        publicSettings={publicSettings}
        collapsed={sidebarCollapsed}
        onLogout={onLogout}
      />
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <Topbar user={user} pageTitle={pageTitle} onSelectTab={onSelectTab} onLogout={onLogout} />
        <main className={`flex-1 ${fullBleed ? 'overflow-hidden flex flex-col' : 'overflow-y-auto p-8'}`}>
          {fullBleed ? (
            children
          ) : (
            <div className="max-w-7xl mx-auto space-y-6">
              {children}
            </div>
          )}
        </main>
      </div>
    </div>
  );
};

