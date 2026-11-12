import React, { useState, useEffect, useCallback } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Layout } from './components/layout/Layout';
import { ToastProvider } from './components/ui/Toast';
import { CustomFieldsProvider } from './fields/CustomFieldContext';
import { LoginView } from './views/LoginView';
import { DashboardView } from './views/DashboardView';
import { ContentTypesView } from './views/ContentTypesView';
import { ContentManagerView } from './views/ContentManagerView';
import { MediaLibraryView } from './views/MediaLibraryView';
import { PluginsView } from './views/PluginsView';
import { ThemesView } from './views/ThemesView';
import { UsersView } from './views/UsersView';
import { getCookie, getStoredToken, setStoredToken } from './api/client';
import {
  authApi,
  contentTypesApi,
  mediaApi,
  pluginsApi,
  themesApi,
} from './api/endpoints';
import type {
  User,
  ContentType,
  MediaAsset,
  PluginEntity,
  ThemeManifest,
} from './types';

const queryClient = new QueryClient();

export function AppContent() {
  const [user, setUser] = useState<User | null>(null);
  const [isAuthChecking, setIsAuthChecking] = useState(true);
  const [currentTab, setCurrentTab] = useState<string>(() => {
    if (typeof window === 'undefined') return 'dashboard';
    return sessionStorage.getItem('cms_active_tab') || 'dashboard';
  });

  useEffect(() => {
    if (typeof window !== 'undefined' && user) {
      sessionStorage.setItem('cms_active_tab', currentTab);
    }
  }, [currentTab, user]);

  // Shared application state
  const [contentTypes, setContentTypes] = useState<ContentType[]>([]);
  const [mediaAssets, setMediaAssets] = useState<MediaAsset[]>([]);
  const [plugins, setPlugins] = useState<PluginEntity[]>([]);
  const [themes, setThemes] = useState<ThemeManifest[]>([]);

  // Initial authentication check
  useEffect(() => {
    const checkAuth = async () => {
      const token = getStoredToken();
      const hasCookieHint = Boolean(getCookie('cms_csrf'));
      if (!token && !hasCookieHint) {
        setIsAuthChecking(false);
        return;
      }
      try {
        const res = await authApi.me();
        const resolved =
          res && typeof res === 'object'
            ? 'data' in res && res.data
              ? res.data
              : 'user' in res && res.user
                ? res.user
                : (res as User)
            : null;
        if (!resolved || (!resolved.id && !resolved.username)) {
          throw new Error('Invalid session user payload');
        }
        setUser(resolved);
      } catch (err) {
        console.warn('Session expired or invalid:', err);
        setStoredToken(null);
        setUser(null);
      } finally {
        setIsAuthChecking(false);
      }
    };
    checkAuth();
  }, []);

  // Fetch core CMS metadata
  const refreshCoreData = useCallback(async () => {
    if (!user) return;
    try {
      const [typesRes, mediaRes, pluginsRes, themesRes] = await Promise.allSettled([
        contentTypesApi.list(),
        mediaApi.list({ limit: 100 }),
        pluginsApi.list(),
        themesApi.list(),
      ]);

      if (typesRes.status === 'fulfilled') setContentTypes(typesRes.value.data || []);
      if (mediaRes.status === 'fulfilled') setMediaAssets(mediaRes.value.data || []);
      if (pluginsRes.status === 'fulfilled') setPlugins(pluginsRes.value.data || []);
      if (themesRes.status === 'fulfilled') setThemes(themesRes.value.data || []);
    } catch (err) {
      console.error('Failed refreshing data:', err);
    }
  }, [user]);

  useEffect(() => {
    if (user) {
      refreshCoreData();
    }
  }, [user, refreshCoreData]);

  const handleLogout = async () => {
    try {
      await authApi.logout();
    } catch {
      // Ignore network errors on logout
    } finally {
      setStoredToken(null);
      setUser(null);
      setCurrentTab('dashboard');
      if (typeof window !== 'undefined') {
        sessionStorage.removeItem('cms_active_tab');
      }
    }
  };

  const handleSelectTab = (tab: string, param?: string) => {
    if (tab === 'content' && param) {
      setCurrentTab(`content:${param}`);
    } else {
      setCurrentTab(tab);
    }
  };

  // Determine topbar page title
  const getPageTitle = () => {
    if (currentTab === 'dashboard') return 'Dashboard';
    if (currentTab === 'content-types') return 'Content Type Builder';
    if (currentTab === 'media') return 'Media Library';
    if (currentTab === 'plugins') return 'Plugins';
    if (currentTab === 'themes') return 'Themes';
    if (currentTab === 'users') return 'Users & Access';
    if (currentTab.startsWith('content:')) {
      const uid = currentTab.replace('content:', '');
      const ct = contentTypes.find((t) => t.uid === uid);
      return ct ? `Content / ${ct.name}` : `Content / ${uid}`;
    }
    return 'Admin Console';
  };

  if (isAuthChecking) {
    return (
      <div
        role="status"
        aria-live="polite"
        aria-busy="true"
        className="min-h-screen w-screen flex items-center justify-center bg-slate-900 text-white"
      >
        <div className="flex flex-col items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-gradient-to-tr from-sky-500 to-indigo-600 animate-pulse flex items-center justify-center font-bold text-lg">
            G
          </div>
          <p className="text-xs text-slate-400 font-mono">Loading Go CMS Console...</p>
        </div>
      </div>
    );
  }

  if (!user) {
    return (
      <LoginView
        onLoginSuccess={(loggedInUser) => {
          setUser(loggedInUser);
        }}
      />
    );
  }

  // Active view router
  let mainContent: React.ReactNode = null;

  if (currentTab === 'dashboard') {
    mainContent = (
      <DashboardView
        contentTypes={contentTypes}
        mediaAssets={mediaAssets}
        plugins={plugins}
        themes={themes}
        onSelectTab={handleSelectTab}
      />
    );
  } else if (currentTab === 'content-types') {
    mainContent = (
      <ContentTypesView
        contentTypes={contentTypes}
        onRefresh={refreshCoreData}
        onOpenContent={(uid) => setCurrentTab(`content:${uid}`)}
      />
    );
  } else if (currentTab.startsWith('content:')) {
    const uid = currentTab.replace('content:', '');
    const activeType = contentTypes.find((t) => t.uid === uid);
    if (activeType) {
      mainContent = (
        <ContentManagerView
          contentType={activeType}
          contentTypes={contentTypes}
          onSelectType={(nextUid) => setCurrentTab(`content:${nextUid}`)}
          onBackToDashboard={() => setCurrentTab('dashboard')}
        />
      );
    } else {
      mainContent = (
        <div className="p-8 text-center text-slate-500">
          Content type "{uid}" not found.
        </div>
      );
    }
  } else if (currentTab === 'media') {
    mainContent = (
      <MediaLibraryView
        mediaAssets={mediaAssets}
        onRefresh={refreshCoreData}
      />
    );
  } else if (currentTab === 'plugins') {
    mainContent = (
      <PluginsView
        plugins={plugins}
        onRefresh={refreshCoreData}
      />
    );
  } else if (currentTab === 'themes') {
    mainContent = <ThemesView themes={themes} />;
  } else if (currentTab === 'users') {
    mainContent = <UsersView />;
  }

  return (
    <Layout
      user={user}
      currentTab={currentTab}
      onSelectTab={handleSelectTab}
      contentTypes={contentTypes}
      onLogout={handleLogout}
      pageTitle={getPageTitle()}
      fullBleed={currentTab === 'content-types' || currentTab.startsWith('content:')}
    >
      {mainContent}
    </Layout>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <CustomFieldsProvider>
          <AppContent />
        </CustomFieldsProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
}
