import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertCircle,
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  ChevronLeft,
  ChevronRight,
  Columns3,
  Copy,
  Database,
  Eye,
  EyeOff,
  FileText,
  GripVertical,
  History,
  MoreHorizontal,
  Pencil,
  Plus,
  RotateCcw,
  Save,
  Search,
  Send,
  SlidersHorizontal,
  Trash2,
} from 'lucide-react';
import { ApiError } from '../api/client';
import { contentTypesApi, entriesApi } from '../api/endpoints';
import { FieldEditor } from '../components/fields/FieldEditor';
import { moveItem } from '../components/fields/contracts';
import { Badge } from '../components/ui/Badge';
import { Button } from '../components/ui/Button';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import { Modal } from '../components/ui/Modal';
import { useToast } from '../components/ui/Toast';
import type {
  ContentEntry,
  ContentField,
  ContentType,
  ContentVersion,
} from '../types';

export interface ContentManagerViewProps {
  contentType: ContentType;
  contentTypes?: ContentType[];
  onSelectType?: (uid: string) => void;
  onBackToDashboard: () => void;
}

export type EffectiveEntryStatus = 'draft' | 'published' | 'modified';

export function getEffectiveStatus(entry: ContentEntry | null): EffectiveEntryStatus {
  if (!entry) return 'draft';
  if (entry.status === 'published') {
    return entry.draft_version > entry.published_version ? 'modified' : 'published';
  }
  return 'draft';
}

function getEntryDisplayTitle(entry: ContentEntry | null, contentType: ContentType): string {
  if (!entry) {
    return contentType.kind === 'single' ? contentType.name : `Create a ${contentType.name}`;
  }
  const fields = contentType.fields || [];
  const priorityField =
    fields.find((f) => ['short_text', 'uid', 'email'].includes(f.field_type)) || fields[0];
  if (priorityField) {
    const val = entry.draft_data?.[priorityField.field_key];
    if (typeof val === 'string' && val.trim()) return val.trim();
  }
  if (entry.slug) return entry.slug;
  return `${contentType.name} #${entry.id.slice(0, 8)}`;
}

function sortJsonValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(sortJsonValue);
  }
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>;
    const sorted: Record<string, unknown> = {};
    for (const key of Object.keys(record).sort()) {
      const next = record[key];
      if (next !== undefined) {
        sorted[key] = sortJsonValue(next);
      }
    }
    return sorted;
  }
  return value;
}

export function canonicalSnapshot(value: unknown): string {
  return JSON.stringify(sortJsonValue(value));
}

function formatCellPreview(field: ContentField, rawValue: unknown): React.ReactNode {
  if (rawValue === null || rawValue === undefined || rawValue === '') {
    return <span className="text-slate-300">—</span>;
  }
  switch (field.field_type) {
    case 'boolean':
      return (
        <span
          className={`inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold ${
            rawValue ? 'bg-emerald-50 text-emerald-700' : 'bg-slate-100 text-slate-500'
          }`}
        >
          {rawValue ? 'True' : 'False'}
        </span>
      );
    case 'media': {
      const count = Array.isArray(rawValue) ? rawValue.length : rawValue ? 1 : 0;
      return (
        <span className="inline-flex items-center rounded bg-purple-50 px-2 py-0.5 text-[11px] font-medium text-purple-700">
          {count} {count === 1 ? 'asset' : 'assets'}
        </span>
      );
    }
    case 'relation_single':
      return (
        <span className="inline-flex items-center rounded bg-sky-50 px-2 py-0.5 font-mono text-[11px] text-sky-700">
          {String(rawValue).slice(0, 8)}
        </span>
      );
    case 'relation_multiple': {
      const count = Array.isArray(rawValue) ? rawValue.length : 0;
      return (
        <span className="inline-flex items-center rounded bg-sky-50 px-2 py-0.5 text-[11px] font-medium text-sky-700">
          {count} linked
        </span>
      );
    }
    case 'repeatable_component':
    case 'dynamic_zone': {
      const count = Array.isArray(rawValue) ? rawValue.length : 0;
      return (
        <span className="text-xs text-slate-600">
          {count} {count === 1 ? 'item' : 'items'}
        </span>
      );
    }
    case 'component':
      return Array.isArray(rawValue) ? (
        <span className="text-xs text-slate-600">
          {rawValue.length} {rawValue.length === 1 ? 'item' : 'items'}
        </span>
      ) : (
        <span className="text-xs text-slate-600">Object</span>
      );
    case 'date':
    case 'datetime': {
      const str = String(rawValue);
      const parsed = Date.parse(str);
      if (Number.isFinite(parsed) && field.field_type === 'datetime') {
        return <span className="text-slate-600">{new Date(parsed).toLocaleString()}</span>;
      }
      return <span className="text-slate-600">{str}</span>;
    }
    case 'rich_text': {
      const stripped = String(rawValue).replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
      return (
        <span className="line-clamp-1 max-w-[220px] text-slate-600">
          {stripped || 'Rich text'}
        </span>
      );
    }
    case 'blocks':
    case 'json':
      return <span className="font-mono text-[11px] text-slate-500">{'{ JSON }'}</span>;
    default: {
      const text = String(rawValue);
      return (
        <span className="block max-w-[240px] truncate text-slate-800">{text}</span>
      );
    }
  }
}

function parseServerErrors(err: unknown): {
  summary: string;
  fields: Record<string, string>;
} {
  const fieldMap: Record<string, string> = {};
  let summary = err instanceof Error ? err.message : 'Request failed.';

  if (err instanceof ApiError && err.data && typeof err.data === 'object') {
    const data = err.data as Record<string, unknown>;
    if (typeof data.field === 'string' && typeof data.error === 'string') {
      fieldMap[data.field] = data.error;
    }
    if (data.fields && typeof data.fields === 'object' && !Array.isArray(data.fields)) {
      for (const [k, v] of Object.entries(data.fields as Record<string, unknown>)) {
        if (typeof v === 'string') fieldMap[k] = v;
      }
    }
  }

  // Fallback parse for Go `fields.FieldError`: `field "title": message`
  const match = summary.match(/^field\s+"([^"]+)":\s*(.+)$/i);
  if (match) {
    fieldMap[match[1]] = match[2];
    summary = match[2];
  }

  return { summary, fields: fieldMap };
}

function buildDefaultFormData(contentType: ContentType): Record<string, unknown> {
  const initial: Record<string, unknown> = {};
  for (const f of contentType.fields || []) {
    if (f.configuration?.read_only || f.configuration?.hidden) {
      continue;
    }
    const def = f.configuration?.default;
    if (def !== undefined) {
      initial[f.field_key] = structuredClone(def);
    } else if (f.field_type === 'boolean') {
      initial[f.field_key] = false;
    } else if (
      f.field_type === 'repeatable_component' ||
      f.field_type === 'dynamic_zone' ||
      f.field_type === 'relation_multiple' ||
      (f.field_type === 'component' && Boolean(f.configuration?.repeatable)) ||
      (f.field_type === 'media' && Boolean(f.configuration?.multiple))
    ) {
      initial[f.field_key] = [];
    } else if (
      f.field_type === 'number' ||
      f.field_type === 'decimal' ||
      f.field_type === 'date' ||
      f.field_type === 'time' ||
      f.field_type === 'datetime' ||
      f.field_type === 'json' ||
      f.field_type === 'blocks' ||
      f.field_type === 'media' ||
      f.field_type === 'relation_single' ||
      f.field_type === 'component'
    ) {
      initial[f.field_key] = null;
    } else {
      initial[f.field_key] = '';
    }
  }
  return initial;
}

export const ContentManagerView: React.FC<ContentManagerViewProps> = ({
  contentType,
  contentTypes,
  onSelectType,
  onBackToDashboard,
}) => {
  const toast = useToast();

  const [definitions, setDefinitions] = useState<ContentType[]>(() =>
    contentTypes?.length ? contentTypes : [contentType],
  );
  const [sidebarSearch, setSidebarSearch] = useState('');

  // List state
  const [entries, setEntries] = useState<ContentEntry[]>([]);
  const [totalEntries, setTotalEntries] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<'' | 'draft' | 'published' | 'modified'>('');

  useEffect(() => {
    if (!searchQuery) {
      setDebouncedSearch('');
      return;
    }
    const timer = window.setTimeout(() => setDebouncedSearch(searchQuery), 250);
    return () => window.clearTimeout(timer);
  }, [searchQuery]);
  const [sortBy, setSortBy] = useState<'updated_at' | 'created_at' | 'slug'>('updated_at');
  const [sortOrder, setSortOrder] = useState<'desc' | 'asc'>('desc');
  const [pageSize, setPageSize] = useState<number>(20);
  const [pageIndex, setPageIndex] = useState<number>(0);

  // Column visibility preference (per content-type in localStorage)
  const storageKey = `cm_cols_${contentType.uid}`;
  const [visibleColumns, setVisibleColumns] = useState<string[]>([]);
  const [showColumnConfig, setShowColumnConfig] = useState(false);

  // Selection & Bulk Actions
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [confirmBulkDelete, setConfirmBulkDelete] = useState(false);

  // Editor state (Single Type opens directly; Collection swaps to Split-Pane Editor)
  const [viewMode, setViewMode] = useState<'list' | 'edit'>(() =>
    contentType.kind === 'single' ? 'edit' : 'list',
  );
  const [documentTab, setDocumentTab] = useState<'draft' | 'published'>('draft');
  const [activeEntry, setActiveEntry] = useState<ContentEntry | null>(null);
  const [formData, setFormData] = useState<Record<string, unknown>>({});
  const [savedFormSnapshot, setSavedFormSnapshot] = useState<string>('{}');
  const [editorError, setEditorError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [clientInvalidPaths, setClientInvalidPaths] = useState<Record<string, string>>({});
  const [isSaving, setIsSaving] = useState(false);
  const [isPublishing, setIsPublishing] = useState(false);

  // Field order & layout mode
  const [fieldOrder, setFieldOrder] = useState<string[]>([]);
  const [layoutMode, setLayoutMode] = useState(false);
  const [draggedField, setDraggedField] = useState<string | null>(null);
  const [isSavingLayout, setIsSavingLayout] = useState(false);

  // Menus & Modals
  const [moreActionsOpen, setMoreActionsOpen] = useState(false);
  const columnMenuRef = useRef<HTMLDivElement | null>(null);
  const moreMenuRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!showColumnConfig && !moreActionsOpen) return;
    const handlePointerDown = (event: MouseEvent) => {
      const target = event.target as Node | null;
      if (showColumnConfig && columnMenuRef.current && target && !columnMenuRef.current.contains(target)) {
        setShowColumnConfig(false);
      }
      if (moreActionsOpen && moreMenuRef.current && target && !moreMenuRef.current.contains(target)) {
        setMoreActionsOpen(false);
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setShowColumnConfig(false);
        setMoreActionsOpen(false);
      }
    };
    document.addEventListener('mousedown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [showColumnConfig, moreActionsOpen]);
  const [deleteTargetIds, setDeleteTargetIds] = useState<string[] | null>(null);
  const [deletingEntries, setDeletingEntries] = useState(false);
  const [pendingNavAction, setPendingNavAction] = useState<(() => void) | null>(null);

  // Version History Modal
  const [isVersionModalOpen, setIsVersionModalOpen] = useState(false);
  const [versionHistory, setVersionHistory] = useState<ContentVersion[]>([]);
  const [isLoadingVersions, setIsLoadingVersions] = useState(false);
  const [previewVersion, setPreviewVersion] = useState<ContentVersion | null>(null);

  // Keep definitions synced
  useEffect(() => {
    if (contentTypes?.length) {
      setDefinitions(contentTypes);
      return;
    }
    let active = true;
    contentTypesApi
      .list()
      .then((res) => {
        if (active) setDefinitions(res.data || [contentType]);
      })
      .catch(() => {
        if (active) setDefinitions([contentType]);
      });
    return () => {
      active = false;
    };
  }, [contentType, contentTypes]);

  // Reset per-content-type state cleanly when switching types
  useEffect(() => {
    setPageIndex(0);
    setSearchQuery('');
    setDebouncedSearch('');
    setStatusFilter('');
    setSelectedIds([]);
    setEditorError(null);
    setFieldErrors({});
    setClientInvalidPaths({});
    setDocumentTab('draft');
    setMoreActionsOpen(false);
    setViewMode(contentType.kind === 'single' ? 'edit' : 'list');
    setActiveEntry(null);
    const defaults = buildDefaultFormData(contentType);
    setFormData(defaults);
    setSavedFormSnapshot(canonicalSnapshot(defaults));

    const keys = (contentType.fields || []).map((f) => f.field_key);
    const cfgOrder = Array.isArray(contentType.settings?.field_order)
      ? contentType.settings.field_order.filter(
          (k: unknown): k is string => typeof k === 'string' && keys.includes(k),
        )
      : [];
    const ordered = [...cfgOrder, ...keys.filter((k) => !cfgOrder.includes(k))];
    setFieldOrder(ordered);

    try {
      const savedCols = localStorage.getItem(`cm_cols_${contentType.uid}`);
      if (savedCols) {
        const parsed = JSON.parse(savedCols);
        if (Array.isArray(parsed)) {
          setVisibleColumns(parsed.filter((k) => keys.includes(k)));
          return;
        }
      }
    } catch {
      // ignore storage errors
    }
    setVisibleColumns(ordered.slice(0, 3));
  }, [contentType]);

  const orderedFields = useMemo(
    () =>
      fieldOrder.length
        ? fieldOrder
            .map((key) => contentType.fields?.find((f) => f.field_key === key))
            .filter((f): f is ContentField => Boolean(f))
        : contentType.fields || [],
    [contentType.fields, fieldOrder],
  );

  const toggleVisibleColumn = (key: string) => {
    setVisibleColumns((current) => {
      const next = current.includes(key)
        ? current.filter((k) => k !== key)
        : [...current, key];
      try {
        localStorage.setItem(storageKey, JSON.stringify(next));
      } catch {
        // ignore storage quota
      }
      return next;
    });
  };

  const fetchEntries = useCallback(async () => {
    setIsLoading(true);
    try {
      // Server status parameter accepts 'draft' or 'published'; modified rows have status='published'
      const serverStatus =
        statusFilter === 'modified' ? 'published' : statusFilter || undefined;
      const res = await entriesApi.list(contentType.uid, {
        limit: pageSize,
        offset: pageIndex * pageSize,
        preview: true,
        search: debouncedSearch || undefined,
        status: serverStatus,
        sort: sortBy,
        order: sortOrder,
      });

      let rows = res.data || [];
      if (statusFilter === 'modified') {
        rows = rows.filter((e) => getEffectiveStatus(e) === 'modified');
      }
      setEntries(rows);
      setTotalEntries(
        statusFilter === 'modified' ? rows.length : res.meta?.total ?? rows.length,
      );

      if (contentType.kind === 'single') {
        const single = rows[0] || null;
        setActiveEntry(single);
        const data = single?.draft_data || buildDefaultFormData(contentType);
        setFormData(data);
        setSavedFormSnapshot(canonicalSnapshot(data));
      }
    } catch (err: unknown) {
      toast.error(
        'Failed to load entries',
        err instanceof Error ? err.message : 'Unable to fetch records.',
      );
    } finally {
      setIsLoading(false);
    }
  }, [
    contentType,
    pageSize,
    pageIndex,
    debouncedSearch,
    statusFilter,
    sortBy,
    sortOrder,
    toast,
  ]);

  useEffect(() => {
    void fetchEntries();
  }, [fetchEntries]);

  const isDirty = useMemo(
    () =>
      viewMode === 'edit' &&
      canonicalSnapshot(formData) !== savedFormSnapshot,
    [viewMode, formData, savedFormSnapshot],
  );

  useEffect(() => {
    if (!isDirty) return;
    const guard = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', guard);
    return () => window.removeEventListener('beforeunload', guard);
  }, [isDirty]);

  const guardNavigation = (action: () => void) => {
    if (isDirty) {
      setPendingNavAction(() => action);
      return;
    }
    action();
  };

  // Persist dragged field order into ContentType.settings.field_order
  const handleReorderField = async (fromKey: string, toKey: string) => {
    if (fromKey === toKey) return;
    const fromIdx = fieldOrder.indexOf(fromKey);
    const toIdx = fieldOrder.indexOf(toKey);
    if (fromIdx < 0 || toIdx < 0) return;
    const next = moveItem(fieldOrder, fromIdx, toIdx);
    setFieldOrder(next);
    setIsSavingLayout(true);
    try {
      await contentTypesApi.update(contentType.uid, {
        name: contentType.name,
        settings: { ...(contentType.settings || {}), field_order: next },
      });
      toast.info('Layout updated', 'Field order saved for this content type.');
    } catch (err: unknown) {
      toast.error(
        'Layout update failed',
        err instanceof Error ? err.message : 'Could not persist field order.',
      );
    } finally {
      setIsSavingLayout(false);
    }
  };

  // Entry Actions
  const openCreateEntry = () => {
    const defaults = buildDefaultFormData(contentType);
    setActiveEntry(null);
    setFormData(defaults);
    setSavedFormSnapshot(canonicalSnapshot(defaults));
    setEditorError(null);
    setFieldErrors({});
    setClientInvalidPaths({});
    setDocumentTab('draft');
    setViewMode('edit');
  };

  const openEditEntry = (entry: ContentEntry) => {
    const data = entry.draft_data || {};
    setActiveEntry(entry);
    setFormData(data);
    setSavedFormSnapshot(canonicalSnapshot(data));
    setEditorError(null);
    setFieldErrors({});
    setClientInvalidPaths({});
    setDocumentTab('draft');
    setViewMode('edit');
  };

  const combinedErrors = useMemo(
    () => ({ ...fieldErrors, ...clientInvalidPaths }),
    [fieldErrors, clientInvalidPaths],
  );

  const saveCurrentDraft = async (): Promise<ContentEntry | null> => {
    setEditorError(null);
    setFieldErrors({});

    if (Object.keys(clientInvalidPaths).length > 0) {
      const firstErr = Object.values(clientInvalidPaths)[0];
      setEditorError(firstErr);
      toast.error('Check field validation', firstErr);
      return null;
    }

    setIsSaving(true);
    try {
      const saved = activeEntry
        ? await entriesApi.update(
            contentType.uid,
            activeEntry.id,
            formData,
            activeEntry.draft_version,
          )
        : await entriesApi.create(contentType.uid, formData);

      setActiveEntry(saved);
      const nextData = saved.draft_data || formData;
      setFormData(nextData);
      setSavedFormSnapshot(canonicalSnapshot(nextData));
      toast.success('Draft saved', `"${getEntryDisplayTitle(saved, contentType)}" saved.`);
      await fetchEntries();
      return saved;
    } catch (err: unknown) {
      const parsed = parseServerErrors(err);
      setFieldErrors(parsed.fields);
      setEditorError(parsed.summary);
      toast.error('Failed to save', parsed.summary);
      return null;
    } finally {
      setIsSaving(false);
    }
  };

  const handlePublishEntry = async () => {
    setIsPublishing(true);
    try {
      let target = activeEntry;
      if (!target || isDirty) {
        target = await saveCurrentDraft();
        if (!target) return;
      }
      const published = await entriesApi.publish(contentType.uid, target.id);
      setActiveEntry(published);
      const nextData = published.draft_data || formData;
      setFormData(nextData);
      setSavedFormSnapshot(canonicalSnapshot(nextData));
      toast.success('Entry published', 'Live content has been updated.');
      await fetchEntries();
    } catch (err: unknown) {
      const parsed = parseServerErrors(err);
      setFieldErrors(parsed.fields);
      setEditorError(parsed.summary);
      toast.error('Publish failed', parsed.summary);
    } finally {
      setIsPublishing(false);
    }
  };

  const handleUnpublishEntry = async (entry: ContentEntry) => {
    setIsPublishing(true);
    try {
      const res = await entriesApi.unpublish(contentType.uid, entry.id);
      if (activeEntry?.id === entry.id) {
        setActiveEntry(res);
        setDocumentTab('draft');
      }
      toast.info('Entry unpublished', 'Removed from live API responses.');
      await fetchEntries();
    } catch (err: unknown) {
      toast.error(
        'Unpublish failed',
        err instanceof Error ? err.message : 'Unable to unpublish entry.',
      );
    } finally {
      setIsPublishing(false);
    }
  };

  const handleDuplicateCurrent = () => {
    const cloned = { ...formData };
    for (const f of contentType.fields || []) {
      if (f.field_type === 'uid') {
        cloned[f.field_key] = '';
      }
    }
    setActiveEntry(null);
    setFormData(cloned);
    setSavedFormSnapshot('{}');
    setDocumentTab('draft');
    setMoreActionsOpen(false);
    toast.info('Entry duplicated into new draft', 'Edit and click Save to create the copy.');
  };

  const handleDeleteConfirmed = async () => {
    if (!deleteTargetIds?.length) return;
    setDeletingEntries(true);
    try {
      const results = await Promise.allSettled(
        deleteTargetIds.map((id) => entriesApi.delete(contentType.uid, id)),
      );
      const ok = results.filter((r) => r.status === 'fulfilled').length;
      const fail = results.length - ok;
      if (ok > 0) {
        toast.success(
          `Deleted ${ok} ${ok === 1 ? 'entry' : 'entries'}`,
          fail > 0 ? `${fail} failed.` : undefined,
        );
      } else {
        toast.error('Delete failed', 'Could not delete the selected entry.');
      }

      if (activeEntry && deleteTargetIds.includes(activeEntry.id)) {
        if (contentType.kind === 'single') {
          setActiveEntry(null);
          const defaults = buildDefaultFormData(contentType);
          setFormData(defaults);
          setSavedFormSnapshot(canonicalSnapshot(defaults));
        } else {
          setViewMode('list');
          setActiveEntry(null);
        }
      }
      setSelectedIds((prev) => prev.filter((id) => !deleteTargetIds.includes(id)));
      setDeleteTargetIds(null);
      setConfirmBulkDelete(false);
      await fetchEntries();
    } finally {
      setDeletingEntries(false);
    }
  };

  const handleBulkAction = async (action: 'publish' | 'unpublish') => {
    if (!selectedIds.length) return;
    setBulkBusy(true);
    try {
      const results = await Promise.allSettled(
        selectedIds.map((id) =>
          action === 'publish'
            ? entriesApi.publish(contentType.uid, id)
            : entriesApi.unpublish(contentType.uid, id),
        ),
      );
      const ok = results.filter((r) => r.status === 'fulfilled').length;
      const fail = results.length - ok;
      if (ok > 0) {
        toast.success(
          `${action === 'publish' ? 'Published' : 'Unpublished'} ${ok} ${ok === 1 ? 'entry' : 'entries'}`,
          fail > 0 ? `${fail} failed.` : undefined,
        );
      } else {
        toast.error(`Bulk ${action} failed`, 'No entries were updated.');
      }
      setSelectedIds([]);
      await fetchEntries();
    } finally {
      setBulkBusy(false);
    }
  };

  // Version history helpers
  const openVersionModal = async (entry: ContentEntry) => {
    setIsVersionModalOpen(true);
    setIsLoadingVersions(true);
    setPreviewVersion(null);
    try {
      const res = await entriesApi.versions(contentType.uid, entry.id);
      setVersionHistory(res.data || []);
    } catch (err: unknown) {
      toast.error(
        'Failed to load versions',
        err instanceof Error ? err.message : 'Unable to fetch version timeline.',
      );
    } finally {
      setIsLoadingVersions(false);
    }
  };

  const restoreVersion = async (versionNumber: number) => {
    if (!activeEntry) return;
    try {
      const restored = await entriesApi.restore(
        contentType.uid,
        activeEntry.id,
        versionNumber,
      );
      setActiveEntry(restored);
      const data = restored.draft_data || {};
      setFormData(data);
      setSavedFormSnapshot(canonicalSnapshot(data));
      setIsVersionModalOpen(false);
      toast.success(`Restored version v${versionNumber}`, 'Loaded into draft.');
      await fetchEntries();
    } catch (err: unknown) {
      toast.error(
        'Restore failed',
        err instanceof Error ? err.message : 'Unable to restore version.',
      );
    }
  };

  // CM Sub-sidebar lists
  const { navCollections, navSingles } = useMemo(() => {
    const q = sidebarSearch.trim().toLowerCase();
    const match = (t: ContentType) =>
      !q || t.name.toLowerCase().includes(q) || t.uid.toLowerCase().includes(q);
    return {
      navCollections: definitions.filter((t) => t.kind === 'collection' && match(t)),
      navSingles: definitions.filter((t) => t.kind === 'single' && match(t)),
    };
  }, [definitions, sidebarSearch]);

  const effectiveStatus = getEffectiveStatus(activeEntry);
  const canViewPublished = Boolean(
    activeEntry && (activeEntry.published_version > 0 || activeEntry.published_data),
  );
  const displayColumns = useMemo(
    () =>
      orderedFields.filter((f) => visibleColumns.includes(f.field_key)),
    [orderedFields, visibleColumns],
  );

  const totalPages = Math.max(1, Math.ceil(totalEntries / pageSize));
  const allVisibleSelected =
    entries.length > 0 && entries.every((e) => selectedIds.includes(e.id));

  const renderStatusBadge = (entry: ContentEntry | null) => {
    if (!entry) return <Badge variant="default">New</Badge>;
    const st = getEffectiveStatus(entry);
    return <Badge variant={st}>{st}</Badge>;
  };

  return (
    <div className="flex min-h-[calc(100vh-4rem)] flex-col lg:flex-row bg-slate-50">
      {/* Left Strapi-Style Content Manager Sub-Sidebar */}
      <aside className="flex w-full shrink-0 flex-col border-b border-slate-200 bg-white lg:w-64 lg:border-r lg:border-b-0">
        <div className="border-b border-slate-200 px-4 py-3.5 space-y-2.5">
          <h2 className="text-sm font-bold text-slate-900">Content Manager</h2>
          <div className="relative">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
            <input
              id="cm-sidebar-search"
              name="cmSidebarSearch"
              type="search"
              value={sidebarSearch}
              onChange={(e) => setSidebarSearch(e.target.value)}
              placeholder="Search models..."
              aria-label="Filter models in Content Manager"
              className="w-full rounded-lg border border-slate-200 bg-slate-50 py-1.5 pr-3 pl-8 text-xs text-slate-800 placeholder:text-slate-400 focus:border-sky-500 focus:bg-white focus:outline-none focus:ring-2 focus:ring-sky-500/20"
            />
          </div>
        </div>

        <nav aria-label="Content Manager models" className="flex-1 overflow-y-auto p-2 space-y-4">
          <div>
            <div className="flex items-center gap-1.5 px-2.5 pt-2 pb-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">
              <Database className="h-3 w-3 text-sky-600" />
              <span>Collection Types ({navCollections.length})</span>
            </div>
            <ul className="space-y-0.5">
              {navCollections.map((t) => {
                const active = t.uid === contentType.uid;
                return (
                  <li key={t.uid}>
                    <button
                      type="button"
                      onClick={() => {
                        if (t.uid === contentType.uid) {
                          guardNavigation(() => setViewMode('list'));
                          return;
                        }
                        if (onSelectType) {
                          guardNavigation(() => onSelectType(t.uid));
                        }
                      }}
                      className={`flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm transition-colors cursor-pointer ${
                        active
                          ? 'bg-sky-50 font-semibold text-sky-700 ring-1 ring-inset ring-sky-200'
                          : 'text-slate-700 hover:bg-slate-100'
                      }`}
                    >
                      <span className="truncate">{t.name}</span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </div>

          <div>
            <div className="flex items-center gap-1.5 px-2.5 pt-1 pb-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">
              <FileText className="h-3 w-3 text-purple-600" />
              <span>Single Types ({navSingles.length})</span>
            </div>
            <ul className="space-y-0.5">
              {navSingles.map((t) => {
                const active = t.uid === contentType.uid;
                return (
                  <li key={t.uid}>
                    <button
                      type="button"
                      onClick={() => {
                        if (t.uid !== contentType.uid && onSelectType) {
                          guardNavigation(() => onSelectType(t.uid));
                        }
                      }}
                      className={`flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm transition-colors cursor-pointer ${
                        active
                          ? 'bg-sky-50 font-semibold text-sky-700 ring-1 ring-inset ring-sky-200'
                          : 'text-slate-700 hover:bg-slate-100'
                      }`}
                    >
                      <span className="truncate">{t.name}</span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </div>
        </nav>
      </aside>

      {/* Main Content Workspace */}
      <section className="flex flex-1 min-w-0 flex-col">
        {viewMode === 'list' && contentType.kind === 'collection' ? (
          /* COLLECTION LISTING VIEW */
          <>
            <header className="border-b border-slate-200 bg-white px-6 py-4">
              <div className="flex flex-wrap items-center justify-between gap-4">
                <div>
                  <div className="flex flex-wrap items-center gap-2.5">
                    <h1 className="text-xl font-bold text-slate-900">
                      {contentType.name}
                    </h1>
                    <Badge variant="collection">Collection</Badge>
                    <code className="rounded border border-slate-200 bg-slate-50 px-2 py-0.5 text-xs font-mono text-slate-600">
                      {contentType.uid}
                    </code>
                  </div>
                  <p className="mt-1 text-xs text-slate-500">
                    {totalEntries} {totalEntries === 1 ? 'entry found' : 'entries found'}
                  </p>
                </div>

                <Button onClick={openCreateEntry}>
                  <Plus className="h-4 w-4" />
                  Create new entry
                </Button>
              </div>

              {/* Action Toolbar: Search, Status Pills, Sort, Column Config */}
              <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
                <div className="flex flex-wrap items-center gap-2">
                  <div className="relative w-64">
                    <Search className="pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
                    <input
                      type="search"
                      name="entry-search"
                      value={searchQuery}
                      onChange={(e) => {
                        setPageIndex(0);
                        setSearchQuery(e.target.value);
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          setDebouncedSearch(e.currentTarget.value);
                        }
                      }}
                      placeholder={`Search ${contentType.name}...`}
                      aria-label={`Search ${contentType.name}`}
                      className="w-full rounded-lg border border-slate-200 bg-slate-50 py-1.5 pr-3 pl-8 text-xs text-slate-900 placeholder:text-slate-400 focus:border-sky-500 focus:bg-white focus:outline-none focus:ring-2 focus:ring-sky-500/20"
                    />
                  </div>

                  {/* Status Filter Pills */}
                  <div className="inline-flex rounded-lg border border-slate-200 bg-slate-50 p-0.5 text-xs">
                    {(
                      [
                        { id: '', label: 'All' },
                        { id: 'draft', label: 'Draft' },
                        { id: 'published', label: 'Published' },
                        { id: 'modified', label: 'Modified' },
                      ] as const
                    ).map((pill) => (
                      <button
                        key={pill.id}
                        type="button"
                        onClick={() => {
                          setPageIndex(0);
                          setStatusFilter(pill.id);
                        }}
                        className={`rounded-md px-2.5 py-1 font-medium transition-colors cursor-pointer ${
                          statusFilter === pill.id
                            ? 'bg-white text-sky-700 shadow-2xs font-semibold'
                            : 'text-slate-600 hover:text-slate-900'
                        }`}
                      >
                        {pill.label}
                      </button>
                    ))}
                  </div>
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  <div className="flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1 text-xs text-slate-600">
                    <SlidersHorizontal className="h-3.5 w-3.5 text-slate-400" />
                    <select
                      id="cm-sort-field"
                      name="sortBy"
                      aria-label="Sort field"
                      value={sortBy}
                      onChange={(e) =>
                        setSortBy(e.target.value as 'updated_at' | 'created_at' | 'slug')
                      }
                      className="bg-transparent font-medium text-slate-700 focus:outline-none cursor-pointer"
                    >
                      <option value="updated_at">Updated at</option>
                      <option value="created_at">Created at</option>
                      <option value="slug">Slug</option>
                    </select>
                    <button
                      type="button"
                      onClick={() => setSortOrder((o) => (o === 'desc' ? 'asc' : 'desc'))}
                      title="Toggle sort direction"
                      className="rounded px-1 font-semibold text-sky-700 hover:bg-sky-50 cursor-pointer"
                    >
                      {sortOrder === 'desc' ? 'DESC' : 'ASC'}
                    </button>
                  </div>

                  {/* Configure View Popover */}
                  <div ref={columnMenuRef} className="relative">
                    <Button
                      variant="outline"
                      size="sm"
                      aria-expanded={showColumnConfig}
                      aria-haspopup="menu"
                      onClick={() => setShowColumnConfig((s) => !s)}
                    >
                      <Columns3 className="h-3.5 w-3.5" />
                      Configure the view
                    </Button>

                    {showColumnConfig && (
                      <div className="absolute right-0 z-30 mt-1.5 w-64 rounded-xl border border-slate-200 bg-white p-3 shadow-xl">
                        <div className="mb-2 flex items-center justify-between border-b border-slate-100 pb-2">
                          <span className="text-xs font-bold text-slate-800">
                            Displayed Table Fields
                          </span>
                          <button
                            type="button"
                            onClick={() => setShowColumnConfig(false)}
                            className="text-xs text-slate-400 hover:text-slate-600 cursor-pointer"
                          >
                            Done
                          </button>
                        </div>
                        <div className="max-h-56 space-y-1 overflow-y-auto">
                          {orderedFields.map((field) => {
                            const checked = visibleColumns.includes(field.field_key);
                            return (
                              <label
                                key={field.field_key}
                                className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs text-slate-700 hover:bg-slate-50"
                              >
                                <input
                                  type="checkbox"
                                  name={`column-${field.field_key}`}
                                  checked={checked}
                                  onChange={() => toggleVisibleColumn(field.field_key)}
                                  className="rounded border-slate-300 text-sky-600 focus:ring-sky-500"
                                />
                                <span className="truncate font-medium">{field.name}</span>
                              </label>
                            );
                          })}
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              </div>

              {/* Bulk Selection Bar */}
              {selectedIds.length > 0 && (
                <div className="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-sky-200 bg-sky-50 px-4 py-2.5 text-xs">
                  <div className="flex items-center gap-2 font-semibold text-sky-900">
                    <span>
                      {selectedIds.length} {selectedIds.length === 1 ? 'entry' : 'entries'} selected
                    </span>
                    <button
                      type="button"
                      onClick={() => setSelectedIds([])}
                      className="font-normal text-sky-700 underline cursor-pointer"
                    >
                      Clear selection
                    </button>
                  </div>

                  <div className="flex flex-wrap items-center gap-2">
                    <Button
                      size="sm"
                      variant="primary"
                      disabled={bulkBusy}
                      onClick={() => void handleBulkAction('publish')}
                    >
                      <Send className="h-3.5 w-3.5" />
                      Publish
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={bulkBusy}
                      onClick={() => void handleBulkAction('unpublish')}
                    >
                      <EyeOff className="h-3.5 w-3.5" />
                      Unpublish
                    </Button>
                    <Button
                      size="sm"
                      variant="destructive"
                      disabled={bulkBusy}
                      onClick={() => {
                        setDeleteTargetIds(selectedIds);
                        setConfirmBulkDelete(true);
                      }}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      Delete
                    </Button>
                  </div>
                </div>
              )}
            </header>

            <div className="flex-1 p-6 space-y-4">
              <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-xs">
                <div className="overflow-x-auto">
                  <table className="w-full border-collapse text-left text-xs">
                    <thead>
                      <tr className="border-b border-slate-200 bg-slate-50/75 text-[11px] font-bold uppercase tracking-wider text-slate-500">
                        <th className="w-10 px-4 py-3">
                          <input
                            id="cm-select-all"
                            name="selectAllEntries"
                            type="checkbox"
                            aria-label="Select all entries on page"
                            checked={allVisibleSelected}
                            onChange={(e) =>
                              setSelectedIds(
                                e.target.checked ? entries.map((item) => item.id) : [],
                              )
                            }
                            className="rounded border-slate-300 text-sky-600 focus:ring-sky-500"
                          />
                        </th>
                        <th className="px-4 py-3">Document</th>
                        {displayColumns.map((col) => (
                          <th key={col.field_key} className="px-4 py-3">
                            {col.name}
                          </th>
                        ))}
                        <th className="px-4 py-3">State</th>
                        <th className="px-4 py-3">Versions</th>
                        <th className="px-4 py-3">Updated</th>
                        <th className="w-24 px-4 py-3 text-right">Actions</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-100 text-slate-700">
                      {isLoading ? (
                        Array.from({ length: 4 }).map((_, idx) => (
                          <tr key={idx} className="animate-pulse">
                            <td colSpan={displayColumns.length + 6} className="px-4 py-4">
                              <div className="h-4 w-full rounded bg-slate-100" />
                            </td>
                          </tr>
                        ))
                      ) : entries.length === 0 ? (
                        <tr>
                          <td
                            colSpan={displayColumns.length + 6}
                            className="px-6 py-14 text-center"
                          >
                            <div className="mx-auto max-w-sm space-y-3">
                              <div className="mx-auto flex h-11 w-11 items-center justify-center rounded-xl bg-slate-100 text-slate-500">
                                <FileText className="h-5 w-5" />
                              </div>
                              <p className="text-sm font-semibold text-slate-800">
                                No entries found
                              </p>
                              <p className="text-xs text-slate-500">
                                {searchQuery || statusFilter
                                  ? 'Try clearing your search or status filter.'
                                  : `Create your first entry for "${contentType.name}".`}
                              </p>
                              {!searchQuery && !statusFilter && (
                                <Button size="sm" onClick={openCreateEntry}>
                                  <Plus className="h-3.5 w-3.5" />
                                  Create new entry
                                </Button>
                              )}
                            </div>
                          </td>
                        </tr>
                      ) : (
                        entries.map((entry) => {
                          const checked = selectedIds.includes(entry.id);
                          return (
                            <tr
                              key={entry.id}
                              onClick={() => openEditEntry(entry)}
                              className={`cursor-pointer transition-colors ${
                                checked ? 'bg-sky-50/50' : 'hover:bg-slate-50'
                              }`}
                            >
                              <td
                                className="px-4 py-3"
                                onClick={(e) => e.stopPropagation()}
                              >
                                <input
                                  type="checkbox"
                                  name={`select-entry-${entry.id}`}
                                  aria-label={`Select entry ${getEntryDisplayTitle(entry, contentType)}`}
                                  checked={checked}
                                  onChange={(e) =>
                                    setSelectedIds((prev) =>
                                      e.target.checked
                                        ? [...prev, entry.id]
                                        : prev.filter((id) => id !== entry.id),
                                    )
                                  }
                                  className="rounded border-slate-300 text-sky-600 focus:ring-sky-500"
                                />
                              </td>
                              <td className="px-4 py-3">
                                <div className="font-semibold text-slate-900 max-w-[220px] truncate">
                                  {getEntryDisplayTitle(entry, contentType)}
                                </div>
                                <code className="text-[11px] font-mono text-slate-400">
                                  {entry.id.slice(0, 8)}
                                </code>
                              </td>
                              {displayColumns.map((col) => (
                                <td key={col.field_key} className="px-4 py-3">
                                  {formatCellPreview(col, entry.draft_data?.[col.field_key])}
                                </td>
                              ))}
                              <td className="px-4 py-3">{renderStatusBadge(entry)}</td>
                              <td className="px-4 py-3 font-mono text-slate-500">
                                v{entry.draft_version} / v{entry.published_version}
                              </td>
                              <td className="px-4 py-3 text-slate-500 whitespace-nowrap">
                                {new Date(entry.updated_at).toLocaleDateString()}
                              </td>
                              <td
                                className="px-4 py-3 text-right whitespace-nowrap"
                                onClick={(e) => e.stopPropagation()}
                              >
                                <div className="inline-flex items-center gap-1">
                                  <button
                                    type="button"
                                    onClick={() => openEditEntry(entry)}
                                    title="Edit entry"
                                    aria-label={`Edit ${getEntryDisplayTitle(entry, contentType)}`}
                                    className="rounded-lg p-1.5 text-slate-500 hover:bg-sky-50 hover:text-sky-700 cursor-pointer"
                                  >
                                    <Pencil className="h-3.5 w-3.5" />
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => setDeleteTargetIds([entry.id])}
                                    title="Delete entry"
                                    aria-label={`Delete ${getEntryDisplayTitle(entry, contentType)}`}
                                    className="rounded-lg p-1.5 text-slate-400 hover:bg-rose-50 hover:text-rose-600 cursor-pointer"
                                  >
                                    <Trash2 className="h-3.5 w-3.5" />
                                  </button>
                                </div>
                              </td>
                            </tr>
                          );
                        })
                      )}
                    </tbody>
                  </table>
                </div>

                {/* Pagination Footer */}
                <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-200 bg-slate-50/70 px-4 py-2.5 text-xs text-slate-600">
                  <div className="flex items-center gap-2">
                    <span>Entries per page:</span>
                    <select
                      id="cm-page-size"
                      name="pageSize"
                      aria-label="Entries per page"
                      value={pageSize}
                      onChange={(e) => {
                        setPageSize(Number(e.target.value));
                        setPageIndex(0);
                      }}
                      className="rounded-md border border-slate-300 bg-white px-2 py-1 font-medium text-slate-700 cursor-pointer"
                    >
                      {[10, 20, 50, 100].map((size) => (
                        <option key={size} value={size}>
                          {size}
                        </option>
                      ))}
                    </select>
                  </div>

                  <div className="flex items-center gap-3">
                    <span>
                      Page {pageIndex + 1} of {totalPages} ({totalEntries} total)
                    </span>
                    <div className="inline-flex items-center gap-1">
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={pageIndex === 0 || isLoading}
                        onClick={() => setPageIndex((p) => Math.max(0, p - 1))}
                      >
                        <ChevronLeft className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={pageIndex + 1 >= totalPages || isLoading}
                        onClick={() => setPageIndex((p) => p + 1)}
                      >
                        <ChevronRight className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </>
        ) : (
          /* STRAPI 5 SPLIT-PANE ENTRY EDITOR (UNIFIED FOR COLLECTION & SINGLE TYPES) */
          <>
            {/* Top Sticky Entry Bar */}
            <header className="sticky top-0 z-20 border-b border-slate-200 bg-white px-6 py-3.5">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-3">
                  <button
                    type="button"
                    onClick={() =>
                      guardNavigation(() =>
                        contentType.kind === 'single'
                          ? onBackToDashboard()
                          : setViewMode('list'),
                      )
                    }
                    className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-100 cursor-pointer"
                  >
                    <ArrowLeft className="h-3.5 w-3.5" />
                    Back
                  </button>

                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h1 className="truncate text-lg font-bold text-slate-900">
                        {getEntryDisplayTitle(activeEntry, contentType)}
                      </h1>
                      {renderStatusBadge(activeEntry)}
                      {isDirty && (
                        <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold text-amber-800">
                          Unsaved draft
                        </span>
                      )}
                    </div>
                    <p className="text-[11px] font-mono text-slate-400">
                      {contentType.name} • {contentType.uid}
                      {activeEntry ? ` • ${activeEntry.id}` : ' • New entry'}
                    </p>
                  </div>
                </div>

                {/* Draft vs Published View Switcher + Layout Toggle + More Actions */}
                <div className="flex flex-wrap items-center gap-2">
                  <div
                    role="tablist"
                    aria-label="Document version state"
                    className="inline-flex rounded-lg bg-slate-100 p-1 text-xs"
                  >
                    <button
                      type="button"
                      role="tab"
                      aria-selected={documentTab === 'draft'}
                      onClick={() => setDocumentTab('draft')}
                      className={`rounded-md px-3 py-1 font-semibold transition-colors cursor-pointer ${
                        documentTab === 'draft'
                          ? 'bg-white text-sky-700 shadow-2xs'
                          : 'text-slate-600 hover:text-slate-900'
                      }`}
                    >
                      Draft
                    </button>
                    <button
                      type="button"
                      role="tab"
                      disabled={!canViewPublished}
                      aria-selected={documentTab === 'published'}
                      onClick={() => canViewPublished && setDocumentTab('published')}
                      title={
                        canViewPublished
                          ? 'Inspect read-only live published content'
                          : 'Publish this entry first to view its live snapshot'
                      }
                      className={`rounded-md px-3 py-1 font-semibold transition-colors ${
                        documentTab === 'published'
                          ? 'bg-white text-emerald-700 shadow-2xs'
                          : canViewPublished
                            ? 'text-slate-600 hover:text-slate-900 cursor-pointer'
                            : 'text-slate-400 cursor-not-allowed opacity-60'
                      }`}
                    >
                      Published
                    </button>
                  </div>

                  <Button
                    size="sm"
                    variant={layoutMode ? 'secondary' : 'outline'}
                    onClick={() => setLayoutMode((m) => !m)}
                    title="Reorder how fields appear in the entry form"
                  >
                    <GripVertical className="h-3.5 w-3.5" />
                    {layoutMode ? 'Done reordering' : 'Edit layout'}
                  </Button>

                  <div ref={moreMenuRef} className="relative">
                    <Button
                      size="sm"
                      variant="outline"
                      aria-label="More entry actions"
                      aria-expanded={moreActionsOpen}
                      aria-haspopup="menu"
                      onClick={() => setMoreActionsOpen((o) => !o)}
                    >
                      <MoreHorizontal className="h-4 w-4" />
                    </Button>

                    {moreActionsOpen && (
                      <div className="absolute right-0 z-30 mt-1.5 w-52 overflow-hidden rounded-xl border border-slate-200 bg-white py-1 shadow-xl text-xs">
                        {activeEntry && (
                          <button
                            type="button"
                            onClick={() => {
                              setMoreActionsOpen(false);
                              void openVersionModal(activeEntry);
                            }}
                            className="flex w-full items-center gap-2 px-3.5 py-2 text-left text-slate-700 hover:bg-slate-50 cursor-pointer"
                          >
                            <History className="h-3.5 w-3.5 text-slate-500" />
                            Version history (v{activeEntry.draft_version})
                          </button>
                        )}
                        {contentType.kind === 'collection' && (
                          <button
                            type="button"
                            onClick={handleDuplicateCurrent}
                            className="flex w-full items-center gap-2 px-3.5 py-2 text-left text-slate-700 hover:bg-slate-50 cursor-pointer"
                          >
                            <Copy className="h-3.5 w-3.5 text-slate-500" />
                            Duplicate into new draft
                          </button>
                        )}
                        {activeEntry?.status === 'published' && (
                          <button
                            type="button"
                            onClick={() => {
                              setMoreActionsOpen(false);
                              void handleUnpublishEntry(activeEntry);
                            }}
                            className="flex w-full items-center gap-2 px-3.5 py-2 text-left text-amber-700 hover:bg-amber-50 cursor-pointer"
                          >
                            <EyeOff className="h-3.5 w-3.5" />
                            Unpublish
                          </button>
                        )}
                        {activeEntry && (
                          <button
                            type="button"
                            onClick={() => {
                              setMoreActionsOpen(false);
                              setDeleteTargetIds([activeEntry.id]);
                            }}
                            className="flex w-full items-center gap-2 border-t border-slate-100 px-3.5 py-2 text-left text-rose-600 hover:bg-rose-50 cursor-pointer"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                            Delete entry
                          </button>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </header>

            {/* Split-Pane Body: Left 70% Canvas + Right Sticky Sidebar */}
            <div className="flex-1 p-6">
              <div className="grid grid-cols-1 items-start gap-6 xl:grid-cols-[1fr_320px]">
                {/* Left Main Form Canvas */}
                <div className="min-w-0 space-y-4">
                  {documentTab === 'published' && activeEntry && (
                    <div className="flex items-center justify-between gap-2 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-xs text-emerald-900">
                      <div className="flex items-center gap-2">
                        <Eye className="h-4 w-4 text-emerald-600" />
                        <span>
                          Viewing read-only live <strong>Published v{activeEntry.published_version}</strong> snapshot.
                        </span>
                      </div>
                      <button
                        type="button"
                        onClick={() => setDocumentTab('draft')}
                        className="font-semibold underline cursor-pointer"
                      >
                        Switch to editable Draft
                      </button>
                    </div>
                  )}

                  {layoutMode && (
                    <div className="flex items-center justify-between gap-2 rounded-xl border border-sky-200 bg-sky-50 px-4 py-2.5 text-xs text-sky-900">
                      <span>
                        Drag field cards or use arrows to reorder the form layout for all <strong>{contentType.name}</strong> entries.
                      </span>
                      <span className="font-semibold text-sky-700">
                        {isSavingLayout ? 'Saving layout...' : 'Auto-saved'}
                      </span>
                    </div>
                  )}

                  {editorError && (
                    <div
                      role="alert"
                      className="flex items-start gap-2.5 rounded-xl border border-rose-200 bg-rose-50 p-4 text-xs text-rose-800"
                    >
                      <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-rose-600" />
                      <div>
                        <p className="font-semibold">Unable to save entry</p>
                        <p className="mt-0.5">{editorError}</p>
                      </div>
                    </div>
                  )}

                  <div className="rounded-xl border border-slate-200 bg-white p-6 shadow-xs">
                    {orderedFields.length === 0 ? (
                      <p className="py-8 text-center text-sm text-slate-500">
                        This content type has no fields configured yet. Add fields in Content-Type Builder.
                      </p>
                    ) : (
                      <div className="grid grid-cols-1 gap-5 md:grid-cols-12">
                        {orderedFields.map((field, index) => {
                          if (field.configuration?.hidden) return null;

                          const prevVisible = orderedFields
                            .slice(0, index)
                            .reverse()
                            .find((f) => !f.configuration?.hidden);
                          const groupHeading =
                            field.configuration?.group &&
                            field.configuration.group !== prevVisible?.configuration?.group
                              ? field.configuration.group
                              : undefined;

                          const colSpan =
                            field.configuration?.layout_width === 'half'
                              ? 'md:col-span-6'
                              : field.configuration?.layout_width === 'third'
                                ? 'md:col-span-4'
                                : field.configuration?.layout_width === 'two-thirds'
                                  ? 'md:col-span-8'
                                  : 'md:col-span-12';

                          const displayData =
                            documentTab === 'published' && activeEntry?.published_data
                              ? activeEntry.published_data
                              : formData;

                          return (
                            <React.Fragment key={field.id || field.field_key}>
                              {groupHeading && (
                                <div className="md:col-span-12 border-b border-slate-100 pt-2 pb-1">
                                  <h3 className="text-xs font-bold uppercase tracking-wider text-slate-500">
                                    {groupHeading}
                                  </h3>
                                </div>
                              )}
                              <div
                                draggable={layoutMode}
                                onDragStart={() => layoutMode && setDraggedField(field.field_key)}
                                onDragEnd={() => setDraggedField(null)}
                                onDragOver={(e) => layoutMode && e.preventDefault()}
                                onDrop={() => {
                                  if (layoutMode && draggedField) {
                                    void handleReorderField(draggedField, field.field_key);
                                  }
                                  setDraggedField(null);
                                }}
                                className={`min-w-0 rounded-lg transition-all ${colSpan} ${
                                  layoutMode
                                    ? 'border border-dashed border-sky-300 bg-sky-50/20 p-3'
                                    : ''
                                } ${draggedField === field.field_key ? 'opacity-50' : ''}`}
                              >
                                {layoutMode && (
                                  <div className="mb-2 flex items-center justify-between text-[11px] text-sky-700">
                                    <span className="flex items-center gap-1 font-semibold">
                                      <GripVertical className="h-3.5 w-3.5 cursor-grab" />
                                      Drag to reorder
                                    </span>
                                    <div className="flex items-center gap-1">
                                      <button
                                        type="button"
                                        disabled={index === 0}
                                        onClick={() =>
                                          void handleReorderField(
                                            field.field_key,
                                            orderedFields[index - 1].field_key,
                                          )
                                        }
                                        aria-label={`Move ${field.name} up`}
                                        className="rounded border border-sky-200 bg-white p-0.5 hover:bg-sky-50 disabled:opacity-30 cursor-pointer"
                                      >
                                        <ArrowUp className="h-3 w-3" />
                                      </button>
                                      <button
                                        type="button"
                                        disabled={index === orderedFields.length - 1}
                                        onClick={() =>
                                          void handleReorderField(
                                            field.field_key,
                                            orderedFields[index + 1].field_key,
                                          )
                                        }
                                        aria-label={`Move ${field.name} down`}
                                        className="rounded border border-sky-200 bg-white p-0.5 hover:bg-sky-50 disabled:opacity-30 cursor-pointer"
                                      >
                                        <ArrowDown className="h-3 w-3" />
                                      </button>
                                    </div>
                                  </div>
                                )}

                                <FieldEditor
                                  key={`${activeEntry?.id || 'new'}-${documentTab}-${field.field_key}`}
                                  field={field}
                                  value={displayData[field.field_key]}
                                  data={displayData}
                                  disabled={documentTab === 'published'}
                                  errors={documentTab === 'draft' ? combinedErrors : undefined}
                                  definitions={definitions}
                                  onDataChange={
                                    documentTab === 'draft'
                                      ? (nextData) => setFormData(nextData)
                                      : undefined
                                  }
                                  onChange={(value) => {
                                    if (documentTab !== 'draft') return;
                                    setFieldErrors((prev) => {
                                      const prefixDot = `${field.field_key}.`;
                                      const prefixBracket = `${field.field_key}[`;
                                      const hasMatch = Object.keys(prev).some(
                                        (k) =>
                                          k === field.field_key ||
                                          k.startsWith(prefixDot) ||
                                          k.startsWith(prefixBracket),
                                      );
                                      if (!hasMatch) return prev;
                                      const next: Record<string, string> = {};
                                      for (const [k, v] of Object.entries(prev)) {
                                        if (
                                          k !== field.field_key &&
                                          !k.startsWith(prefixDot) &&
                                          !k.startsWith(prefixBracket)
                                        ) {
                                          next[k] = v;
                                        }
                                      }
                                      return next;
                                    });
                                    setFormData((current) => ({
                                      ...current,
                                      [field.field_key]: value,
                                    }));
                                  }}
                                  onValidityChange={(path, message) =>
                                    setClientInvalidPaths((current) => {
                                      const next = { ...current };
                                      if (message) next[path] = message;
                                      else delete next[path];
                                      return next;
                                    })
                                  }
                                />
                              </div>
                            </React.Fragment>
                          );
                        })}
                      </div>
                    )}
                  </div>
                </div>

                {/* Right Sticky Status, Publish & Metadata Sidebar */}
                <aside className="space-y-4 xl:sticky xl:top-20">
                  {/* Publishing Actions Card */}
                  <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-xs space-y-4">
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-bold uppercase tracking-wider text-slate-500">
                        Status
                      </span>
                      {renderStatusBadge(activeEntry)}
                    </div>

                    <div className="rounded-lg bg-slate-50 px-3 py-2 text-xs text-slate-600">
                      {isDirty ? (
                        <span className="font-medium text-amber-700">
                          Unsaved edits in working draft.
                        </span>
                      ) : activeEntry ? (
                        effectiveStatus === 'modified' ? (
                          <span>
                            Draft v{activeEntry.draft_version} has unpublished changes vs live v{activeEntry.published_version}.
                          </span>
                        ) : effectiveStatus === 'published' ? (
                          <span className="text-emerald-700">
                            Published version matches latest draft.
                          </span>
                        ) : (
                          <span>Saved as draft (not yet published).</span>
                        )
                      ) : (
                        <span>Not yet saved to database.</span>
                      )}
                    </div>

                    <div className="space-y-2">
                      <Button
                        className="w-full"
                        variant={isDirty || !activeEntry ? 'primary' : 'outline'}
                        disabled={
                          documentTab === 'published' ||
                          (!isDirty && Boolean(activeEntry)) ||
                          isPublishing
                        }
                        isLoading={isSaving}
                        onClick={() => void saveCurrentDraft()}
                      >
                        <Save className="h-4 w-4" />
                        Save
                      </Button>

                      <Button
                        className="w-full"
                        variant="primary"
                        disabled={
                          documentTab === 'published' ||
                          isSaving ||
                          (!isDirty && effectiveStatus === 'published')
                        }
                        isLoading={isPublishing}
                        onClick={() => void handlePublishEntry()}
                      >
                        <Send className="h-4 w-4" />
                        {effectiveStatus === 'modified' ? 'Publish changes' : 'Publish'}
                      </Button>

                      {activeEntry?.status === 'published' && (
                        <Button
                          className="w-full"
                          variant="outline"
                          disabled={isPublishing || isSaving}
                          onClick={() => void handleUnpublishEntry(activeEntry)}
                        >
                          <EyeOff className="h-4 w-4 text-amber-600" />
                          Unpublish
                        </Button>
                      )}
                    </div>
                  </div>

                  {/* Information Metadata Card */}
                  <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-xs space-y-3">
                    <h3 className="border-b border-slate-100 pb-2 text-xs font-bold uppercase tracking-wider text-slate-500">
                      Information
                    </h3>
                    {activeEntry ? (
                      <dl className="space-y-2.5 text-xs">
                        <div className="flex items-center justify-between gap-2">
                          <dt className="text-slate-500">Document ID</dt>
                          <dd className="font-mono text-slate-800 truncate max-w-[160px]" title={activeEntry.id}>
                            {activeEntry.id}
                          </dd>
                        </div>
                        {activeEntry.slug && (
                          <div className="flex items-center justify-between gap-2">
                            <dt className="text-slate-500">Slug</dt>
                            <dd className="font-mono text-slate-800 truncate max-w-[160px]">
                              {activeEntry.slug}
                            </dd>
                          </div>
                        )}
                        <div className="flex items-center justify-between gap-2">
                          <dt className="text-slate-500">Draft version</dt>
                          <dd className="font-mono font-semibold text-slate-800">
                            v{activeEntry.draft_version}
                          </dd>
                        </div>
                        <div className="flex items-center justify-between gap-2">
                          <dt className="text-slate-500">Live version</dt>
                          <dd className="font-mono font-semibold text-slate-800">
                            {activeEntry.published_version > 0
                              ? `v${activeEntry.published_version}`
                              : 'Unpublished'}
                          </dd>
                        </div>
                        <div className="flex items-center justify-between gap-2">
                          <dt className="text-slate-500">Created</dt>
                          <dd className="text-slate-700">
                            {new Date(activeEntry.created_at).toLocaleString()}
                          </dd>
                        </div>
                        <div className="flex items-center justify-between gap-2">
                          <dt className="text-slate-500">Updated</dt>
                          <dd className="text-slate-700">
                            {new Date(activeEntry.updated_at).toLocaleString()}
                          </dd>
                        </div>
                      </dl>
                    ) : (
                      <p className="text-xs text-slate-500">
                        Metadata will appear once this entry is saved.
                      </p>
                    )}
                  </div>

                  {/* Version History & Danger Actions */}
                  {activeEntry && (
                    <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-xs space-y-2">
                      <Button
                        variant="outline"
                        size="sm"
                        className="w-full justify-start"
                        onClick={() => void openVersionModal(activeEntry)}
                      >
                        <History className="h-4 w-4 text-slate-500" />
                        Version History (v{activeEntry.draft_version})
                      </Button>

                      <Button
                        variant="ghost"
                        size="sm"
                        className="w-full justify-start text-rose-600 hover:bg-rose-50 hover:text-rose-700"
                        onClick={() => setDeleteTargetIds([activeEntry.id])}
                      >
                        <Trash2 className="h-4 w-4" />
                        Delete entry
                      </Button>
                    </div>
                  )}
                </aside>
              </div>
            </div>
          </>
        )}
      </section>

      {/* Version History Timeline Modal */}
      <Modal
        isOpen={isVersionModalOpen}
        onClose={() => setIsVersionModalOpen(false)}
        title={`Version History — ${getEntryDisplayTitle(activeEntry, contentType)}`}
        description="Inspect past draft & published revisions and restore safely into your working draft."
        maxWidth="2xl"
      >
        <div className="space-y-4">
          {isLoadingVersions ? (
            <div className="py-10 text-center text-sm text-slate-400">
              Loading revision snapshots...
            </div>
          ) : versionHistory.length === 0 ? (
            <div className="py-10 text-center text-sm text-slate-500">
              No recorded revision snapshots yet.
            </div>
          ) : (
            <div className="divide-y divide-slate-100 rounded-xl border border-slate-200 bg-white">
              {versionHistory.map((v) => {
                const isPreviewing = previewVersion?.id === v.id;
                return (
                  <div key={v.id} className="p-4 space-y-2.5">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2.5">
                        <span className="rounded-md bg-slate-100 px-2 py-0.5 font-mono text-xs font-bold text-slate-800">
                          v{v.version_number}
                        </span>
                        <Badge
                          variant={v.action === 'published' ? 'published' : 'draft'}
                        >
                          {v.action}
                        </Badge>
                        <span className="text-xs text-slate-500">
                          {new Date(v.created_at).toLocaleString()}
                        </span>
                      </div>

                      <div className="flex items-center gap-2">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() =>
                            setPreviewVersion(isPreviewing ? null : v)
                          }
                        >
                          <Eye className="h-3.5 w-3.5" />
                          {isPreviewing ? 'Hide JSON' : 'Preview'}
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => void restoreVersion(v.version_number)}
                        >
                          <RotateCcw className="h-3.5 w-3.5" />
                          Restore
                        </Button>
                      </div>
                    </div>

                    {isPreviewing && (
                      <pre className="max-h-56 overflow-auto rounded-lg bg-slate-900 p-3 font-mono text-[11px] text-slate-100">
                        {JSON.stringify(v.snapshot, null, 2)}
                      </pre>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </Modal>

      {/* Delete Entry / Bulk Delete Confirmation */}
      <ConfirmDialog
        isOpen={Boolean(deleteTargetIds?.length)}
        onClose={() => {
          if (!deletingEntries) {
            setDeleteTargetIds(null);
            setConfirmBulkDelete(false);
          }
        }}
        onConfirm={handleDeleteConfirmed}
        variant="destructive"
        isLoading={deletingEntries}
        title={
          confirmBulkDelete && (deleteTargetIds?.length || 0) > 1
            ? `Delete ${deleteTargetIds?.length} entries?`
            : 'Delete this entry?'
        }
        description="This deletes the selected content record(s) from the collection. This action cannot be undone."
        confirmLabel="Delete permanently"
      />

      {/* Unsaved Entry Changes Guard */}
      <ConfirmDialog
        isOpen={Boolean(pendingNavAction)}
        onClose={() => setPendingNavAction(null)}
        onConfirm={() => {
          const fn = pendingNavAction;
          setPendingNavAction(null);
          fn?.();
        }}
        variant="destructive"
        title="Discard unsaved entry changes?"
        description="You have unsaved edits in the current entry form. Leaving now will discard them."
        confirmLabel="Discard and leave"
        cancelLabel="Stay and edit"
      />
    </div>
  );
};
