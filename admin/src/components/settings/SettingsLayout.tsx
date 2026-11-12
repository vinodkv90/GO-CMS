import React, { useEffect, useState } from 'react';
import { ChevronRight } from 'lucide-react';
import { Button } from '../ui/Button';
import { ConfirmDialog } from '../ui/ConfirmDialog';
import { Badge } from '../ui/Badge';

export interface SettingsNavItem {
  id: string;
  label: string;
  icon?: React.ReactNode;
  badge?: string;
}

export interface SettingsNavGroup {
  id: string;
  label: string;
  items: SettingsNavItem[];
}

export interface SettingsLayoutProps {
  groups: SettingsNavGroup[];
  activeSection: string;
  onSelectSection: (sectionId: string) => void;
  title: string;
  description?: string;
  breadcrumbGroup?: string;
  isDirty?: boolean;
  isSaving?: boolean;
  canSave?: boolean;
  saveLabel?: string;
  onSave?: () => void;
  onDiscard?: () => void;
  headerActions?: React.ReactNode;
  children: React.ReactNode;
}

export const SettingsLayout: React.FC<SettingsLayoutProps> = ({
  groups,
  activeSection,
  onSelectSection,
  title,
  description,
  breadcrumbGroup = 'Settings',
  isDirty = false,
  isSaving = false,
  canSave = true,
  saveLabel = 'Save changes',
  onSave,
  onDiscard,
  headerActions,
  children,
}) => {
  const [pendingSection, setPendingSection] = useState<string | null>(null);

  useEffect(() => {
    if (!isDirty || typeof window === 'undefined') return;
    const handleBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [isDirty]);

  const handleNavClick = (sectionId: string) => {
    if (sectionId === activeSection) return;
    if (isDirty) {
      setPendingSection(sectionId);
      return;
    }
    onSelectSection(sectionId);
  };

  return (
    <div className="flex flex-1 h-full min-h-0 overflow-hidden bg-slate-50">
      {/* Strapi-inspired Settings Left Navigation Rail */}
      <aside
        aria-label="Settings sections"
        className="w-64 shrink-0 bg-white border-r border-slate-200/80 overflow-y-auto px-3 py-5 space-y-6 select-none"
      >
        <div className="px-3">
          <p className="text-xs font-extrabold uppercase tracking-wider text-slate-900">
            Settings
          </p>
          <p className="text-[11px] text-slate-500 mt-0.5">
            Platform configuration &amp; access
          </p>
        </div>

        <nav className="space-y-5">
          {groups
            .filter((g) => g.items.length > 0)
            .map((group) => (
              <div key={group.id} className="space-y-1">
                <p className="px-3 text-[10px] font-bold uppercase tracking-wider text-slate-400 mb-1.5">
                  {group.label}
                </p>
                {group.items.map((item) => {
                  const isActive = item.id === activeSection;
                  return (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => handleNavClick(item.id)}
                      aria-current={isActive ? 'page' : undefined}
                      className={`w-full flex items-center justify-between gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                        isActive
                          ? 'bg-sky-50 text-sky-700 font-semibold border border-sky-200/70'
                          : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900 border border-transparent'
                      }`}
                    >
                      <span className="flex items-center gap-2.5 truncate">
                        {item.icon && <span className="shrink-0 opacity-85">{item.icon}</span>}
                        <span className="truncate">{item.label}</span>
                      </span>
                      {item.badge && (
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 text-slate-600 font-semibold shrink-0">
                          {item.badge}
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            ))}
        </nav>
      </aside>

      {/* Main Content Area */}
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        {/* Sticky Settings Header */}
        <div className="bg-white border-b border-slate-200/80 px-8 py-4 flex flex-wrap items-center justify-between gap-4 shrink-0">
          <div className="space-y-1 min-w-0">
            <div className="flex items-center gap-1.5 text-[11px] font-medium text-slate-500">
              <span>Settings</span>
              <ChevronRight className="w-3 h-3" />
              <span>{breadcrumbGroup}</span>
              <ChevronRight className="w-3 h-3" />
              <span className="text-slate-700 font-semibold">{title}</span>
            </div>
            <div className="flex items-center gap-2.5">
              <h1 className="text-xl font-bold text-slate-900 tracking-tight truncate">
                {title}
              </h1>
              {isDirty && <Badge variant="warning">Unsaved changes</Badge>}
            </div>
            {description && <p className="text-xs text-slate-500">{description}</p>}
          </div>

          <div className="flex items-center gap-2.5 shrink-0">
            {headerActions}
            {onDiscard && isDirty && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={isSaving}
                onClick={onDiscard}
              >
                Discard
              </Button>
            )}
            {onSave && (
              <Button
                type="button"
                variant="primary"
                size="sm"
                isLoading={isSaving}
                disabled={!isDirty || !canSave}
                onClick={onSave}
              >
                {saveLabel}
              </Button>
            )}
          </div>
        </div>

        {/* Scrollable Inner Canvas */}
        <div className="flex-1 overflow-y-auto p-8">
          <div className="max-w-5xl mx-auto space-y-6">{children}</div>
        </div>
      </div>

      <ConfirmDialog
        isOpen={pendingSection !== null}
        onClose={() => setPendingSection(null)}
        onConfirm={() => {
          const target = pendingSection;
          setPendingSection(null);
          if (onDiscard) onDiscard();
          if (target) onSelectSection(target);
        }}
        title="Discard unsaved changes?"
        description="You have unsaved configuration changes in this section. Leaving now will discard them."
        confirmLabel="Discard & leave"
        cancelLabel="Stay here"
        variant="destructive"
      />
    </div>
  );
};

