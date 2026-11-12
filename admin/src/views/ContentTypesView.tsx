import React, { useEffect, useMemo, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  Box,
  Database,
  ExternalLink,
  FileText,
  FolderOpen,
  GripVertical,
  Layers,
  Pencil,
  Plus,
  RotateCcw,
  Save,
  Search,
  Settings2,
  Trash2,
  AlertCircle,
} from 'lucide-react';
import { contentTypesApi } from '../api/endpoints';
import { BuilderDialog } from '../components/builder/BuilderDialog';
import { FieldEditor } from '../components/builder/FieldEditor';
import { moveItem, slugify } from '../components/fields/contracts';
import { Badge } from '../components/ui/Badge';
import { Button } from '../components/ui/Button';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import { Input } from '../components/ui/Input';
import { Modal } from '../components/ui/Modal';
import { useToast } from '../components/ui/Toast';
import {
  describeFieldSummary,
  FieldTypeIcon,
  getFieldVisual,
} from '../fields/presentation';
import { FIELD_REGISTRY, orderedFields } from '../fields/registry';
import type { ContentField, ContentKind, ContentType } from '../types';

export interface ContentTypesViewProps {
  contentTypes: ContentType[];
  onRefresh: () => void | Promise<void>;
  onOpenContent: (uid: string) => void;
}

const UID_REGEX = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const COMPONENT_UID_REGEX = /^[a-z0-9]+(?:-[a-z0-9]+)*\.[a-z0-9]+(?:-[a-z0-9]+)*$/;

function canonicalFieldSnapshot(fields: ContentField[]) {
  return fields.map((field) => ({
    name: field.name,
    field_key: field.field_key,
    field_type: field.field_type,
    configuration: field.configuration || {},
  }));
}

function componentCategoryOf(type: ContentType): string {
  const fromSettings = typeof type.settings?.category === 'string' ? type.settings.category.trim() : '';
  if (fromSettings) return fromSettings;
  const dotIdx = type.uid.indexOf('.');
  if (dotIdx > 0) return type.uid.slice(0, dotIdx);
  return 'shared';
}

export const ContentTypesView: React.FC<ContentTypesViewProps> = ({
  contentTypes,
  onRefresh,
  onOpenContent,
}) => {
  const toast = useToast();

  // Selection & navigation guard state
  const [selectedUid, setSelectedUid] = useState<string | null>(() => {
    const firstCol = contentTypes.find((t) => t.kind === 'collection');
    const firstSingle = contentTypes.find((t) => t.kind === 'single');
    const firstComp = contentTypes.find((t) => t.kind === 'component');
    return (firstCol || firstSingle || firstComp)?.uid ?? null;
  });
  const [sidebarSearch, setSidebarSearch] = useState('');
  const [pendingSwitchUid, setPendingSwitchUid] = useState<string | null>(null);

  // Staged fields state for selected content type
  const [stagedFields, setStagedFields] = useState<ContentField[]>([]);
  const [draggingKey, setDraggingKey] = useState<string | null>(null);
  const [savingSchema, setSavingSchema] = useState(false);

  // Create Type Modal state
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [createKind, setCreateKind] = useState<ContentKind>('collection');
  const [createName, setCreateName] = useState('');
  const [createCategory, setCreateCategory] = useState('shared');
  const [createUid, setCreateUid] = useState('');
  const [createUidManual, setCreateUidManual] = useState(false);
  const [createDescription, setCreateDescription] = useState('');
  const [createError, setCreateError] = useState('');
  const [creatingType, setCreatingType] = useState(false);

  // Edit Type Settings Modal state
  const [editSettingsOpen, setEditSettingsOpen] = useState(false);
  const [settingsName, setSettingsName] = useState('');
  const [settingsDescription, setSettingsDescription] = useState('');
  const [settingsError, setSettingsError] = useState('');
  const [savingSettings, setSavingSettings] = useState(false);

  // Delete Type Confirm state
  const [deleteTypeTarget, setDeleteTypeTarget] = useState<ContentType | null>(null);
  const [deletingType, setDeletingType] = useState(false);

  // Field Builder Dialog state
  const [fieldDialogMode, setFieldDialogMode] = useState<'closed' | 'add' | 'edit'>('closed');
  const [editingFieldKey, setEditingFieldKey] = useState<string | null>(null);

  const selectedType = useMemo(
    () => contentTypes.find((t) => t.uid === selectedUid) || null,
    [contentTypes, selectedUid],
  );

  const savedFields = useMemo(
    () => (selectedType ? orderedFields(selectedType) : []),
    [selectedType],
  );

  // Auto-select first available type if current selection disappeared or was initially null
  useEffect(() => {
    if (selectedType) return;
    const fallback =
      contentTypes.find((t) => t.kind === 'collection') ||
      contentTypes.find((t) => t.kind === 'single') ||
      contentTypes.find((t) => t.kind === 'component') ||
      null;
    setSelectedUid(fallback?.uid ?? null);
  }, [contentTypes, selectedType]);

  // Reset staged copy whenever selected type or its saved fields refresh from server
  const savedSnapshotJson = useMemo(
    () => JSON.stringify(canonicalFieldSnapshot(savedFields)),
    [savedFields],
  );

  useEffect(() => {
    setStagedFields(savedFields.map((f) => ({ ...f, configuration: { ...f.configuration } })));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedType?.uid, savedSnapshotJson]);

  const stagedSnapshotJson = useMemo(
    () => JSON.stringify(canonicalFieldSnapshot(stagedFields)),
    [stagedFields],
  );

  const hasUnsavedChanges = Boolean(selectedType && stagedSnapshotJson !== savedSnapshotJson);

  // Diff breakdown for informative UX pills
  const diffSummary = useMemo(() => {
    const savedMap = new Map(savedFields.map((f) => [f.field_key, f]));
    const stagedMap = new Map(stagedFields.map((f) => [f.field_key, f]));

    let added = 0;
    let modified = 0;
    let removed = 0;

    for (const f of stagedFields) {
      const orig = savedMap.get(f.field_key);
      if (!orig) {
        added += 1;
      } else if (
        JSON.stringify(canonicalFieldSnapshot([f])) !==
        JSON.stringify(canonicalFieldSnapshot([orig]))
      ) {
        modified += 1;
      }
    }
    for (const f of savedFields) {
      if (!stagedMap.has(f.field_key)) {
        removed += 1;
      }
    }

    const reordered =
      added === 0 &&
      modified === 0 &&
      removed === 0 &&
      JSON.stringify(stagedFields.map((f) => f.field_key)) !==
        JSON.stringify(savedFields.map((f) => f.field_key));

    return { added, modified, removed, reordered, savedMap };
  }, [savedFields, stagedFields]);

  // Browser tab unload guard when staged edits exist
  useEffect(() => {
    if (!hasUnsavedChanges) return;
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [hasUnsavedChanges]);

  // Sidebar grouped content types filtered by search
  const { collections, singles, componentGroups, allComponentCategories } = useMemo(() => {
    const query = sidebarSearch.trim().toLowerCase();
    const matches = (t: ContentType) =>
      !query ||
      t.name.toLowerCase().includes(query) ||
      t.uid.toLowerCase().includes(query);

    const cols = contentTypes.filter((t) => t.kind === 'collection' && matches(t));
    const sngs = contentTypes.filter((t) => t.kind === 'single' && matches(t));
    const comps = contentTypes.filter((t) => t.kind === 'component' && matches(t));

    const groupMap = new Map<string, ContentType[]>();
    const allCats = new Set<string>(['shared', 'sections']);

    for (const comp of contentTypes) {
      if (comp.kind === 'component') {
        allCats.add(componentCategoryOf(comp));
      }
    }

    for (const comp of comps) {
      const cat = componentCategoryOf(comp);
      const list = groupMap.get(cat) || [];
      list.push(comp);
      groupMap.set(cat, list);
    }

    const sortedGroups = Array.from(groupMap.entries()).sort(([a], [b]) => a.localeCompare(b));
    return {
      collections: cols,
      singles: sngs,
      componentGroups: sortedGroups,
      allComponentCategories: Array.from(allCats).sort(),
    };
  }, [contentTypes, sidebarSearch]);

  const requestSelectType = (uid: string) => {
    if (uid === selectedUid) return;
    if (hasUnsavedChanges) {
      setPendingSwitchUid(uid);
      return;
    }
    setSelectedUid(uid);
  };

  const openCreateModal = (kind: ContentKind) => {
    setCreateKind(kind);
    setCreateName('');
    setCreateCategory('shared');
    setCreateUid('');
    setCreateUidManual(false);
    setCreateDescription('');
    setCreateError('');
    setCreateModalOpen(true);
  };

  const computeAutoUid = (kind: ContentKind, name: string, category: string) => {
    const base = slugify(name);
    if (kind === 'component') {
      const cat = slugify(category) || 'shared';
      return base ? `${cat}.${base}` : `${cat}.`;
    }
    return base;
  };

  const handleCreateNameChange = (value: string) => {
    setCreateName(value);
    if (!createUidManual) {
      setCreateUid(computeAutoUid(createKind, value, createCategory));
    }
  };

  const handleCreateCategoryChange = (value: string) => {
    setCreateCategory(value);
    if (!createUidManual && createKind === 'component') {
      setCreateUid(computeAutoUid('component', createName, value));
    }
  };

  const handleCreateKindChange = (kind: ContentKind) => {
    setCreateKind(kind);
    if (!createUidManual) {
      setCreateUid(computeAutoUid(kind, createName, createCategory));
    }
  };

  const handleCreateSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    const trimmedName = createName.trim();
    const trimmedUid = createUid.trim().toLowerCase();

    if (!trimmedName) {
      setCreateError('Display name is required.');
      return;
    }
    if (createKind === 'component') {
      if (!COMPONENT_UID_REGEX.test(trimmedUid)) {
        setCreateError('Component API ID must use "category.name" format (lowercase letters, numbers, hyphens).');
        return;
      }
    } else if (!UID_REGEX.test(trimmedUid)) {
      setCreateError('API ID must contain lowercase letters, numbers, and hyphens only (e.g. "blog-post").');
      return;
    }

    setCreatingType(true);
    setCreateError('');
    try {
      const settings: Record<string, unknown> = {};
      if (createDescription.trim()) {
        settings.description = createDescription.trim();
      }
      if (createKind === 'component') {
        const cat = slugify(createCategory) || trimmedUid.split('.')[0] || 'shared';
        settings.category = cat;
      }

      await contentTypesApi.create({
        name: trimmedName,
        uid: trimmedUid,
        kind: createKind,
        settings: Object.keys(settings).length > 0 ? settings : undefined,
      });

      setCreateModalOpen(false);
      await onRefresh();
      setSelectedUid(trimmedUid);
      setFieldDialogMode('add');
      setEditingFieldKey(null);
      toast.success(
        `${createKind === 'component' ? 'Component' : 'Content type'} created`,
        `Add your first field to "${trimmedName}".`,
      );
    } catch (err: unknown) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create schema type.');
    } finally {
      setCreatingType(false);
    }
  };

  const openEditSettings = () => {
    if (!selectedType) return;
    setSettingsName(selectedType.name);
    setSettingsDescription(
      typeof selectedType.settings?.description === 'string'
        ? selectedType.settings.description
        : '',
    );
    setSettingsError('');
    setEditSettingsOpen(true);
  };

  const handleSaveSettings = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!selectedType) return;
    const trimmedName = settingsName.trim();
    if (!trimmedName) {
      setSettingsError('Display name is required.');
      return;
    }
    setSavingSettings(true);
    setSettingsError('');
    try {
      const nextSettings: Record<string, unknown> = { ...(selectedType.settings || {}) };
      if (settingsDescription.trim()) {
        nextSettings.description = settingsDescription.trim();
      } else {
        delete nextSettings.description;
      }
      await contentTypesApi.update(selectedType.uid, {
        name: trimmedName,
        settings: nextSettings,
      });
      await onRefresh();
      setEditSettingsOpen(false);
      toast.success('Settings updated', `"${trimmedName}" settings saved.`);
    } catch (err: unknown) {
      setSettingsError(err instanceof Error ? err.message : 'Failed to update settings.');
    } finally {
      setSavingSettings(false);
    }
  };

  const handleConfirmDeleteType = async () => {
    if (!deleteTypeTarget) return;
    setDeletingType(true);
    try {
      await contentTypesApi.delete(deleteTypeTarget.uid);
      const removedName = deleteTypeTarget.name;
      setDeleteTypeTarget(null);
      if (selectedUid === deleteTypeTarget.uid) {
        setSelectedUid(null);
      }
      await onRefresh();
      toast.success('Deleted', `"${removedName}" has been removed.`);
    } catch (err: unknown) {
      toast.error(
        'Cannot delete type',
        err instanceof Error ? err.message : 'Ensure entries or references are cleared first.',
      );
      setDeleteTypeTarget(null);
    } finally {
      setDeletingType(false);
    }
  };

  // Field staging actions
  const handleStageFieldSave = async (
    nextField: ContentField,
    options?: { addAnother?: boolean },
  ) => {
    setStagedFields((current) => {
      if (fieldDialogMode === 'edit' && editingFieldKey) {
        return current.map((f) => (f.field_key === editingFieldKey ? nextField : f));
      }
      return [...current, nextField];
    });
    if (!options?.addAnother) {
      setFieldDialogMode('closed');
      setEditingFieldKey(null);
      toast.info('Change staged', 'Click Save in the header to persist your schema.');
    } else {
      toast.success(`Staged "${nextField.name}"`, 'Pick the next field type.');
    }
  };

  const handleRemoveStagedField = (fieldKey: string) => {
    setStagedFields((current) => current.filter((f) => f.field_key !== fieldKey));
  };

  const handleMoveField = (fromIdx: number, toIdx: number) => {
    setStagedFields((current) => moveItem(current, fromIdx, toIdx));
  };

  const handleDiscardChanges = () => {
    setStagedFields(savedFields.map((f) => ({ ...f, configuration: { ...f.configuration } })));
    toast.info('Changes discarded', 'Schema reverted to last saved definition.');
  };

  const handleSaveSchema = async () => {
    if (!selectedType || !hasUnsavedChanges) return;
    setSavingSchema(true);
    try {
      const savedMap = new Map(savedFields.map((f) => [f.field_key, f]));
      const stagedMap = new Map(stagedFields.map((f) => [f.field_key, f]));

      // 1. Update modified existing fields first (keeps UID/relation target updates valid before deletions)
      for (const field of stagedFields) {
        const orig = savedMap.get(field.field_key);
        if (
          orig &&
          JSON.stringify(canonicalFieldSnapshot([field])) !==
            JSON.stringify(canonicalFieldSnapshot([orig]))
        ) {
          await contentTypesApi.updateField(selectedType.uid, field.field_key, field);
        }
      }

      // 2. Add newly staged fields (non-UID first so linked UID target_field exists even if reordered)
      const fieldsToAdd = stagedFields.filter((field) => !savedMap.has(field.field_key));
      fieldsToAdd.sort((a, b) => (a.field_type === 'uid' ? 1 : 0) - (b.field_type === 'uid' ? 1 : 0));
      for (const field of fieldsToAdd) {
        await contentTypesApi.addField(selectedType.uid, field);
      }

      // 3. Delete removed fields
      for (const orig of savedFields) {
        if (!stagedMap.has(orig.field_key)) {
          await contentTypesApi.deleteField(selectedType.uid, orig.field_key);
        }
      }

      // 4. Persist field order if changed or fields changed
      const newOrder = stagedFields.map((f) => f.field_key);
      const oldOrder = savedFields.map((f) => f.field_key);
      if (JSON.stringify(newOrder) !== JSON.stringify(oldOrder)) {
        await contentTypesApi.update(selectedType.uid, {
          name: selectedType.name,
          settings: {
            ...(selectedType.settings || {}),
            field_order: newOrder,
          },
        });
      }

      await onRefresh();
      toast.success('Schema saved', `Saved ${stagedFields.length} fields for "${selectedType.name}".`);
    } catch (err: unknown) {
      toast.error(
        'Failed to save schema',
        err instanceof Error ? err.message : 'Some schema operations failed; refreshing state.',
      );
      await onRefresh();
    } finally {
      setSavingSchema(false);
    }
  };

  // Owner snapshot passed to FieldEditor reflects staged fields (for UID targets & key uniqueness)
  const stagedOwner = useMemo<ContentType | null>(
    () => (selectedType ? { ...selectedType, fields: stagedFields } : null),
    [selectedType, stagedFields],
  );

  const editingField = useMemo(
    () =>
      fieldDialogMode === 'edit' && editingFieldKey
        ? stagedFields.find((f) => f.field_key === editingFieldKey)
        : undefined,
    [fieldDialogMode, editingFieldKey, stagedFields],
  );

  const renderSidebarGroup = (
    title: string,
    items: ContentType[],
    kind: ContentKind,
    icon: React.ReactNode,
    createLabel: string,
  ) => (
    <div className="space-y-1">
      <div className="flex items-center justify-between px-3 pt-4 pb-1">
        <div className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-slate-500">
          {icon}
          <span>{title}</span>
          <span className="ml-0.5 rounded-full bg-slate-200/80 px-1.5 py-0 text-[10px] font-semibold text-slate-600">
            {items.length}
          </span>
        </div>
        <button
          type="button"
          onClick={() => openCreateModal(kind)}
          title={createLabel}
          aria-label={createLabel}
          className="rounded-md p-1 text-sky-600 transition-colors hover:bg-sky-50 hover:text-sky-700 cursor-pointer"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>
      {items.length === 0 ? (
        <button
          type="button"
          onClick={() => openCreateModal(kind)}
          className="mx-2 flex w-[calc(100%-1rem)] items-center gap-1.5 rounded-lg border border-dashed border-slate-200 px-3 py-2 text-xs text-slate-500 transition-colors hover:border-sky-300 hover:bg-sky-50/40 hover:text-sky-700 cursor-pointer"
        >
          <Plus className="h-3.5 w-3.5" />
          <span>{createLabel}</span>
        </button>
      ) : (
        <ul className="space-y-0.5 px-2">
          {items.map((item) => {
            const active = item.uid === selectedUid;
            return (
              <li key={item.uid}>
                <button
                  type="button"
                  onClick={() => requestSelectType(item.uid)}
                  className={`flex w-full items-center justify-between rounded-lg px-2.5 py-2 text-left text-sm transition-colors cursor-pointer ${
                    active
                      ? 'bg-sky-50 font-semibold text-sky-700 ring-1 ring-inset ring-sky-200'
                      : 'text-slate-700 hover:bg-slate-100'
                  }`}
                >
                  <span className="truncate">{item.name}</span>
                  <span className="ml-2 shrink-0 text-[11px] text-slate-400">
                    {item.fields?.length ?? 0}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );

  return (
    <div className="flex flex-1 h-full min-h-0 flex-col overflow-y-auto lg:flex-row lg:overflow-hidden bg-slate-50">
      {/* Left Strapi-Style Builder Sub-Sidebar */}
      <aside className="flex w-full shrink-0 flex-col border-b border-slate-200 bg-white lg:w-72 lg:min-h-0 lg:border-r lg:border-b-0">
        <div className="border-b border-slate-200 px-4 py-3.5 space-y-2.5">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-bold text-slate-900">Content-Type Builder</h2>
            <span className="text-xs text-slate-400">{contentTypes.length} models</span>
          </div>
          <div className="relative">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
            <input
              id="ctb-sidebar-search"
              name="ctbSidebarSearch"
              type="search"
              value={sidebarSearch}
              onChange={(e) => setSidebarSearch(e.target.value)}
              placeholder="Search types & components..."
              aria-label="Search content types and components"
              className="w-full rounded-lg border border-slate-200 bg-slate-50 py-1.5 pr-3 pl-8 text-xs text-slate-800 placeholder:text-slate-400 focus:border-sky-500 focus:bg-white focus:outline-none focus:ring-2 focus:ring-sky-500/20"
            />
          </div>
        </div>

        <nav
          aria-label="Content-Type Builder schema navigation"
          className="flex-1 overflow-y-auto pb-6"
        >
          {renderSidebarGroup(
            'Collection Types',
            collections,
            'collection',
            <Database className="h-3.5 w-3.5 text-sky-600" />,
            'Create new collection type',
          )}

          {renderSidebarGroup(
            'Single Types',
            singles,
            'single',
            <FileText className="h-3.5 w-3.5 text-purple-600" />,
            'Create new single type',
          )}

          {/* Components grouped by category */}
          <div className="space-y-1">
            <div className="flex items-center justify-between px-3 pt-4 pb-1">
              <div className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-slate-500">
                <Layers className="h-3.5 w-3.5 text-indigo-600" />
                <span>Components</span>
                <span className="ml-0.5 rounded-full bg-slate-200/80 px-1.5 py-0 text-[10px] font-semibold text-slate-600">
                  {componentGroups.reduce((acc, [, list]) => acc + list.length, 0)}
                </span>
              </div>
              <button
                type="button"
                onClick={() => openCreateModal('component')}
                title="Create new component"
                aria-label="Create new component"
                className="rounded-md p-1 text-sky-600 transition-colors hover:bg-sky-50 hover:text-sky-700 cursor-pointer"
              >
                <Plus className="h-3.5 w-3.5" />
              </button>
            </div>

            {componentGroups.length === 0 ? (
              <button
                type="button"
                onClick={() => openCreateModal('component')}
                className="mx-2 flex w-[calc(100%-1rem)] items-center gap-1.5 rounded-lg border border-dashed border-slate-200 px-3 py-2 text-xs text-slate-500 transition-colors hover:border-sky-300 hover:bg-sky-50/40 hover:text-sky-700 cursor-pointer"
              >
                <Plus className="h-3.5 w-3.5" />
                <span>Create new component</span>
              </button>
            ) : (
              <div className="space-y-2 px-2">
                {componentGroups.map(([category, items]) => (
                  <div key={category} className="space-y-0.5">
                    <div className="flex items-center gap-1.5 px-2.5 pt-1 text-[11px] font-semibold text-slate-400">
                      <FolderOpen className="h-3 w-3" />
                      <span className="uppercase tracking-wide">{category}</span>
                    </div>
                    <ul className="space-y-0.5">
                      {items.map((comp) => {
                        const active = comp.uid === selectedUid;
                        return (
                          <li key={comp.uid}>
                            <button
                              type="button"
                              onClick={() => requestSelectType(comp.uid)}
                              className={`flex w-full items-center justify-between rounded-lg py-1.5 pr-2.5 pl-6 text-left text-sm transition-colors cursor-pointer ${
                                active
                                  ? 'bg-sky-50 font-semibold text-sky-700 ring-1 ring-inset ring-sky-200'
                                  : 'text-slate-700 hover:bg-slate-100'
                              }`}
                            >
                              <span className="truncate">{comp.name}</span>
                              <span className="ml-2 shrink-0 text-[11px] text-slate-400">
                                {comp.fields?.length ?? 0}
                              </span>
                            </button>
                          </li>
                        );
                      })}
                    </ul>
                  </div>
                ))}
              </div>
            )}
          </div>
        </nav>
      </aside>

      {/* Right Workspace Region */}
      <section className="flex flex-1 min-w-0 lg:min-h-0 flex-col lg:overflow-hidden">
        {!selectedType ? (
          <div className="flex flex-1 items-center justify-center p-8">
            <div className="max-w-md rounded-2xl border border-slate-200 bg-white p-8 text-center shadow-xs">
              <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-sky-50 text-sky-600">
                <Box className="h-7 w-7" />
              </div>
              <h3 className="text-lg font-bold text-slate-900">
                Build your first Content Type
              </h3>
              <p className="mt-2 text-sm text-slate-600">
                Define Collection Types for repeatable entries, Single Types for global pages, or reusable Components for structured blocks.
              </p>
              <div className="mt-6 flex flex-wrap justify-center gap-2.5">
                <Button onClick={() => openCreateModal('collection')}>
                  <Plus className="h-4 w-4" /> Collection Type
                </Button>
                <Button variant="outline" onClick={() => openCreateModal('single')}>
                  <Plus className="h-4 w-4" /> Single Type
                </Button>
                <Button variant="outline" onClick={() => openCreateModal('component')}>
                  <Plus className="h-4 w-4" /> Component
                </Button>
              </div>
            </div>
          </div>
        ) : (
          <>
            {/* Top Model Action Bar */}
            <header className="shrink-0 border-b border-slate-200 bg-white px-6 py-4">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2.5">
                    <h1 className="truncate text-xl font-bold text-slate-900">
                      {selectedType.name}
                    </h1>
                    <Badge variant={selectedType.kind}>
                      {selectedType.kind === 'collection'
                        ? 'Collection Type'
                        : selectedType.kind === 'single'
                          ? 'Single Type'
                          : `Component • ${componentCategoryOf(selectedType)}`}
                    </Badge>
                    <code className="rounded-md border border-slate-200 bg-slate-50 px-2 py-0.5 text-xs font-mono text-slate-600">
                      {selectedType.uid}
                    </code>
                    {selectedType.kind !== 'component' && (
                      <span className="inline-flex items-center rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700">
                        Draft &amp; Publish enabled
                      </span>
                    )}
                  </div>
                  {typeof selectedType.settings?.description === 'string' &&
                    selectedType.settings.description.trim() !== '' && (
                      <p className="text-sm text-slate-500">
                        {selectedType.settings.description}
                      </p>
                    )}
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  {selectedType.kind !== 'component' && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => onOpenContent(selectedType.uid)}
                    >
                      <ExternalLink className="h-3.5 w-3.5" />
                      Manage Entries
                    </Button>
                  )}

                  <Button
                    variant="outline"
                    size="sm"
                    onClick={openEditSettings}
                    aria-label="Edit model settings"
                  >
                    <Settings2 className="h-3.5 w-3.5" />
                    Edit
                  </Button>

                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setDeleteTypeTarget(selectedType)}
                    className="text-rose-600 hover:bg-rose-50 hover:text-rose-700"
                    aria-label="Delete content type"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    Delete
                  </Button>

                  <div className="mx-1 hidden h-5 w-px bg-slate-200 sm:block" />

                  {hasUnsavedChanges && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={savingSchema}
                      onClick={handleDiscardChanges}
                    >
                      <RotateCcw className="h-3.5 w-3.5" />
                      Discard
                    </Button>
                  )}

                  <Button
                    variant={hasUnsavedChanges ? 'primary' : 'outline'}
                    size="sm"
                    isLoading={savingSchema}
                    disabled={!hasUnsavedChanges}
                    onClick={handleSaveSchema}
                  >
                    <Save className="h-3.5 w-3.5" />
                    Save
                  </Button>

                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => {
                      setEditingFieldKey(null);
                      setFieldDialogMode('add');
                    }}
                  >
                    <Plus className="h-3.5 w-3.5" />
                    Add another field
                  </Button>
                </div>
              </div>

              {hasUnsavedChanges && (
                <div
                  role="status"
                  className="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3.5 py-2 text-xs text-amber-900"
                >
                  <div className="flex items-center gap-2">
                    <AlertCircle className="h-4 w-4 shrink-0 text-amber-600" />
                    <span className="font-medium">You have unsaved schema changes:</span>
                    <span className="text-amber-800">
                      {[
                        diffSummary.added > 0 ? `+${diffSummary.added} added` : null,
                        diffSummary.modified > 0 ? `${diffSummary.modified} modified` : null,
                        diffSummary.removed > 0 ? `-${diffSummary.removed} removed` : null,
                        diffSummary.reordered ? 'field order updated' : null,
                      ]
                        .filter(Boolean)
                        .join(' • ')}
                    </span>
                  </div>
                  <span className="text-amber-700">Click Save to persist to the database.</span>
                </div>
              )}
            </header>

            {/* Fields Listing Card */}
            <div className="flex-1 lg:overflow-y-auto p-6">
              {stagedFields.length === 0 ? (
                <div className="rounded-xl border border-slate-200 bg-white p-12 text-center shadow-xs">
                  <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-xl bg-sky-50 text-sky-600">
                    <Plus className="h-6 w-6" />
                  </div>
                  <h2 className="text-base font-bold text-slate-900">
                    Add your first field to {selectedType.name}
                  </h2>
                  <p className="mx-auto mt-1 max-w-sm text-sm text-slate-500">
                    Choose from text, rich text, numbers, media, relations, repeatable components, dynamic zones, and more.
                  </p>
                  <div className="mt-5">
                    <Button
                      onClick={() => {
                        setEditingFieldKey(null);
                        setFieldDialogMode('add');
                      }}
                    >
                      <Plus className="h-4 w-4" />
                      Add first field
                    </Button>
                  </div>
                </div>
              ) : (
                <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-xs">
                  <div className="flex items-center justify-between border-b border-slate-200 bg-slate-50/70 px-4 py-2.5">
                    <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                      Fields ({stagedFields.length})
                    </span>
                    <span className="text-xs text-slate-400">
                      Drag rows or use arrows to reorder entry layout
                    </span>
                  </div>

                  <div className="overflow-x-auto">
                    <table className="w-full border-collapse text-left text-sm">
                      <thead>
                        <tr className="border-b border-slate-200 bg-slate-50/40 text-xs font-semibold uppercase tracking-wider text-slate-500">
                          <th className="w-14 px-3 py-2.5">Order</th>
                          <th className="px-4 py-2.5">Name</th>
                          <th className="px-4 py-2.5">Type</th>
                          <th className="px-4 py-2.5">Configuration</th>
                          <th className="px-4 py-2.5">Flags</th>
                          <th className="w-28 px-4 py-2.5 text-right">Actions</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-slate-100">
                        {stagedFields.map((field, index) => {
                          const orig = diffSummary.savedMap.get(field.field_key);
                          const isNew = !orig;
                          const isEdited = Boolean(
                            orig &&
                              JSON.stringify(canonicalFieldSnapshot([field])) !==
                                JSON.stringify(canonicalFieldSnapshot([orig])),
                          );
                          const visual = getFieldVisual(field.field_type);
                          const cfg = field.configuration || {};
                          const summary = describeFieldSummary(field, contentTypes);
                          const supported = Boolean(FIELD_REGISTRY[field.field_type]);

                          return (
                            <tr
                              key={field.field_key}
                              draggable
                              onDragStart={() => setDraggingKey(field.field_key)}
                              onDragOver={(e) => {
                                e.preventDefault();
                                if (draggingKey && draggingKey !== field.field_key) {
                                  const fromIdx = stagedFields.findIndex(
                                    (f) => f.field_key === draggingKey,
                                  );
                                  if (fromIdx >= 0 && fromIdx !== index) {
                                    handleMoveField(fromIdx, index);
                                  }
                                }
                              }}
                              onDragEnd={() => setDraggingKey(null)}
                              className={`group transition-colors ${
                                draggingKey === field.field_key
                                  ? 'bg-sky-50/80'
                                  : 'hover:bg-slate-50/80'
                              }`}
                            >
                              <td className="px-3 py-3">
                                <div className="flex items-center gap-1 text-slate-400">
                                  <span
                                    className="cursor-grab text-slate-300 group-hover:text-slate-500"
                                    title="Drag to reorder"
                                  >
                                    <GripVertical className="h-4 w-4" />
                                  </span>
                                  <div className="flex flex-col">
                                    <button
                                      type="button"
                                      disabled={index === 0}
                                      onClick={() => handleMoveField(index, index - 1)}
                                      aria-label={`Move ${field.name} up`}
                                      className="rounded p-0.5 hover:bg-slate-200/70 hover:text-slate-700 disabled:opacity-30 cursor-pointer"
                                    >
                                      <ArrowUp className="h-3 w-3" />
                                    </button>
                                    <button
                                      type="button"
                                      disabled={index === stagedFields.length - 1}
                                      onClick={() => handleMoveField(index, index + 1)}
                                      aria-label={`Move ${field.name} down`}
                                      className="rounded p-0.5 hover:bg-slate-200/70 hover:text-slate-700 disabled:opacity-30 cursor-pointer"
                                    >
                                      <ArrowDown className="h-3 w-3" />
                                    </button>
                                  </div>
                                </div>
                              </td>

                              <td className="px-4 py-3">
                                <div className="flex items-center gap-3">
                                  <FieldTypeIcon fieldType={field.field_type} size="sm" />
                                  <div className="min-w-0">
                                    <div className="flex items-center gap-2">
                                      <span className="font-semibold text-slate-900">
                                        {field.name}
                                      </span>
                                      {isNew && (
                                        <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-semibold text-emerald-700">
                                          New
                                        </span>
                                      )}
                                      {isEdited && (
                                        <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-semibold text-amber-800">
                                          Edited
                                        </span>
                                      )}
                                    </div>
                                    <code className="text-xs font-mono text-slate-500">
                                      {field.field_key}
                                    </code>
                                  </div>
                                </div>
                              </td>

                              <td className="px-4 py-3 text-slate-700">
                                <span className="font-medium">{visual.shortLabel}</span>
                                {!supported && (
                                  <span className="ml-1.5 rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold text-amber-800">
                                    Custom
                                  </span>
                                )}
                              </td>

                              <td className="px-4 py-3 text-xs text-slate-600">{summary}</td>

                              <td className="px-4 py-3">
                                <div className="flex flex-wrap gap-1">
                                  {cfg.required && (
                                    <Badge variant="warning">Required</Badge>
                                  )}
                                  {cfg.unique && (
                                    <Badge variant="info">Unique</Badge>
                                  )}
                                  {cfg.private && (
                                    <Badge variant="default">Private</Badge>
                                  )}
                                  {cfg.read_only && (
                                    <Badge variant="default">Read-only</Badge>
                                  )}
                                  {cfg.hidden && (
                                    <Badge variant="default">Hidden</Badge>
                                  )}
                                  {cfg.layout_width && cfg.layout_width !== 'full' && (
                                    <Badge variant="default">{cfg.layout_width}</Badge>
                                  )}
                                </div>
                              </td>

                              <td className="px-4 py-3 text-right">
                                <div className="inline-flex items-center gap-1">
                                  <button
                                    type="button"
                                    onClick={() => {
                                      setEditingFieldKey(field.field_key);
                                      setFieldDialogMode('edit');
                                    }}
                                    title={`Configure ${field.name}`}
                                    aria-label={`Configure ${field.name}`}
                                    className="rounded-lg p-1.5 text-slate-500 transition-colors hover:bg-sky-50 hover:text-sky-700 cursor-pointer"
                                  >
                                    <Pencil className="h-4 w-4" />
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => handleRemoveStagedField(field.field_key)}
                                    title={`Delete ${field.name}`}
                                    aria-label={`Delete ${field.name}`}
                                    className="rounded-lg p-1.5 text-slate-400 transition-colors hover:bg-rose-50 hover:text-rose-600 cursor-pointer"
                                  >
                                    <Trash2 className="h-4 w-4" />
                                  </button>
                                </div>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>

                  {/* Strapi-style bottom action bar to add another field */}
                  <button
                    type="button"
                    onClick={() => {
                      setEditingFieldKey(null);
                      setFieldDialogMode('add');
                    }}
                    className="flex w-full items-center justify-center gap-2 border-t border-slate-200 bg-slate-50/70 py-3 text-xs font-semibold text-sky-700 transition-colors hover:bg-sky-50 cursor-pointer"
                  >
                    <Plus className="h-4 w-4" />
                    Add another field to {selectedType.name}
                  </button>
                </div>
              )}
            </div>
          </>
        )}
      </section>

      {/* Create Content Type / Single Type / Component Modal */}
      <Modal
        isOpen={createModalOpen}
        onClose={() => !creatingType && setCreateModalOpen(false)}
        title={
          createKind === 'collection'
            ? 'Create a collection type'
            : createKind === 'single'
              ? 'Create a single type'
              : 'Create a component'
        }
        description="Configure your schema display name and API identifier."
        maxWidth="lg"
      >
        <form onSubmit={handleCreateSubmit} className="space-y-4">
          {createError && (
            <div
              role="alert"
              className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700"
            >
              {createError}
            </div>
          )}

          {/* Kind Switcher */}
          <div className="grid grid-cols-3 gap-2 rounded-lg bg-slate-100 p-1">
            {(
              [
                { id: 'collection', label: 'Collection Type' },
                { id: 'single', label: 'Single Type' },
                { id: 'component', label: 'Component' },
              ] as const
            ).map((option) => (
              <button
                key={option.id}
                type="button"
                onClick={() => handleCreateKindChange(option.id)}
                className={`rounded-md py-1.5 text-xs font-semibold transition-colors cursor-pointer ${
                  createKind === option.id
                    ? 'bg-white text-sky-700 shadow-xs'
                    : 'text-slate-600 hover:text-slate-900'
                }`}
              >
                {option.label}
              </button>
            ))}
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <Input
              label="Display name"
              required
              autoFocus
              value={createName}
              onChange={(e) => handleCreateNameChange(e.target.value)}
              placeholder={
                createKind === 'component'
                  ? 'e.g. SEO Metadata'
                  : createKind === 'single'
                    ? 'e.g. Global Settings'
                    : 'e.g. Blog Post'
              }
            />

            {createKind === 'component' ? (
              <div className="space-y-1.5">
                <Input
                  label="Category"
                  required
                  value={createCategory}
                  onChange={(e) => handleCreateCategoryChange(e.target.value)}
                  placeholder="shared"
                />
                <div className="flex flex-wrap gap-1">
                  {allComponentCategories.map((cat) => (
                    <button
                      key={cat}
                      type="button"
                      onClick={() => handleCreateCategoryChange(cat)}
                      className={`rounded px-1.5 py-0.5 text-[11px] font-medium cursor-pointer ${
                        createCategory === cat
                          ? 'bg-indigo-100 text-indigo-700'
                          : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
                      }`}
                    >
                      {cat}
                    </button>
                  ))}
                </div>
              </div>
            ) : (
              <Input
                label="API ID (UID)"
                required
                value={createUid}
                onChange={(e) => {
                  setCreateUidManual(true);
                  setCreateUid(e.target.value.toLowerCase());
                }}
                helperText="Used in REST endpoints: /api/v1/content/<uid>"
              />
            )}
          </div>

          {createKind === 'component' && (
            <Input
              label="Component API ID"
              required
              value={createUid}
              onChange={(e) => {
                setCreateUidManual(true);
                setCreateUid(e.target.value.toLowerCase());
              }}
              helperText="Format: category.name (lowercase letters, numbers, hyphens)"
            />
          )}

          <div>
            <label
              htmlFor="ctb-create-description"
              className="mb-1 block text-xs font-semibold text-slate-700"
            >
              Description (optional)
            </label>
            <textarea
              id="ctb-create-description"
              name="description"
              rows={2}
              value={createDescription}
              onChange={(e) => setCreateDescription(e.target.value)}
              placeholder="Briefly describe what this schema stores..."
              className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-sky-500 focus:outline-none focus:ring-2 focus:ring-sky-500/20"
            />
          </div>

          <div className="flex justify-end gap-2 border-t border-slate-100 pt-4">
            <Button
              type="button"
              variant="outline"
              disabled={creatingType}
              onClick={() => setCreateModalOpen(false)}
            >
              Cancel
            </Button>
            <Button type="submit" isLoading={creatingType}>
              Continue to add fields
            </Button>
          </div>
        </form>
      </Modal>

      {/* Edit Model Settings Modal */}
      <Modal
        isOpen={editSettingsOpen}
        onClose={() => !savingSettings && setEditSettingsOpen(false)}
        title={`Edit ${selectedType?.name || 'model'} settings`}
        maxWidth="md"
      >
        <form onSubmit={handleSaveSettings} className="space-y-4">
          {settingsError && (
            <div
              role="alert"
              className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700"
            >
              {settingsError}
            </div>
          )}

          <Input
            label="Display name"
            required
            value={settingsName}
            onChange={(e) => setSettingsName(e.target.value)}
          />

          <Input
            label="API ID"
            disabled
            value={selectedType?.uid || ''}
            helperText="API identifier cannot be modified after creation."
          />

          <div>
            <label
              htmlFor="ctb-edit-description"
              className="mb-1 block text-xs font-semibold text-slate-700"
            >
              Description
            </label>
            <textarea
              id="ctb-edit-description"
              name="description"
              rows={3}
              value={settingsDescription}
              onChange={(e) => setSettingsDescription(e.target.value)}
              className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-sky-500 focus:outline-none focus:ring-2 focus:ring-sky-500/20"
            />
          </div>

          <div className="flex justify-end gap-2 border-t border-slate-100 pt-4">
            <Button
              type="button"
              variant="outline"
              disabled={savingSettings}
              onClick={() => setEditSettingsOpen(false)}
            >
              Cancel
            </Button>
            <Button type="submit" isLoading={savingSettings}>
              Save settings
            </Button>
          </div>
        </form>
      </Modal>

      {/* Add / Edit Field Dialog */}
      {fieldDialogMode !== 'closed' && stagedOwner && (
        <BuilderDialog
          title={
            fieldDialogMode === 'edit' && editingField
              ? `Edit ${editingField.name}`
              : `Add field to ${stagedOwner.name}`
          }
          onClose={() => {
            setFieldDialogMode('closed');
            setEditingFieldKey(null);
          }}
        >
          <FieldEditor
            owner={stagedOwner}
            contentTypes={contentTypes}
            field={editingField}
            onSave={handleStageFieldSave}
            onCancel={() => {
              setFieldDialogMode('closed');
              setEditingFieldKey(null);
            }}
          />
        </BuilderDialog>
      )}

      {/* Delete Type Confirmation Dialog */}
      <ConfirmDialog
        isOpen={Boolean(deleteTypeTarget)}
        onClose={() => !deletingType && setDeleteTypeTarget(null)}
        onConfirm={handleConfirmDeleteType}
        variant="destructive"
        isLoading={deletingType}
        title={`Delete "${deleteTypeTarget?.name || ''}"?`}
        description="This permanently removes the schema definition. Content types with existing entries or incoming references cannot be deleted."
        confirmLabel="Delete permanently"
      />

      {/* Unsaved Changes Navigation Guard Dialog */}
      <ConfirmDialog
        isOpen={Boolean(pendingSwitchUid)}
        onClose={() => setPendingSwitchUid(null)}
        onConfirm={() => {
          const target = pendingSwitchUid;
          setPendingSwitchUid(null);
          if (target) setSelectedUid(target);
        }}
        variant="destructive"
        title="Discard unsaved schema changes?"
        description="You have staged field changes that have not been saved yet. Switching models will discard those staged changes."
        confirmLabel="Discard and switch"
        cancelLabel="Stay and save"
      />
    </div>
  );
};
