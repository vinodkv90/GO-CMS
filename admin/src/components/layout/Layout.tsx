import React from 'react';
import { Sidebar } from './Sidebar';
import { Topbar } from './Topbar';
import type { User, ContentType } from '../../types';

export interface LayoutProps {
  user: User | null;
  currentTab: string;
  onSelectTab: (tab: string, param?: string) => void;
  contentTypes: ContentType[];
  onLogout: () => void;
  pageTitle: string;
  children: React.ReactNode;
}

export const Layout: React.FC<LayoutProps> = ({
  user,
  currentTab,
  onSelectTab,
  contentTypes,
  onLogout,
  pageTitle,
  children,
}) => {
  return (
    <div className="flex h-screen w-screen overflow-hidden bg-slate-50 font-sans antialiased text-slate-800">
      <Sidebar
        currentTab={currentTab}
        onSelectTab={onSelectTab}
        contentTypes={contentTypes}
        onLogout={onLogout}
      />
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <Topbar user={user} pageTitle={pageTitle} />
        <main className="flex-1 overflow-y-auto p-8">
          <div className="max-w-7xl mx-auto space-y-6">
            {children}
          </div>
        </main>
      </div>
    </div>
  );
};

