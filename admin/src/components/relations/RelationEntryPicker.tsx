import React, { useEffect, useMemo, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  Check,
  ChevronLeft,
  ChevronRight,
  GripVertical,
  Search,
  X,
} from 'lucide-react';
import { entriesApi } from '../../api/endpoints';
import type { ContentEntry } from '../../types';

interface RelationEntryPickerProps {
  contentTypeUid: string;
  multiple: boolean;
  value: string[];
  onChange: (ids: string[]) => void;
  disabled?: boolean;
}

const labelFor = (entry: ContentEntry): string => {
  for (const key of ['title', 'name', 'label']) {
    const value = entry.draft_data?.[key];
    if (typeof value === 'string' && value.trim()) return value;
  }
  const first = Object.values(entry.draft_data || {}).find(
    (value): value is string => typeof value === 'string' && value.trim().length > 0,
  );
  return first || entry.slug || entry.id;
};

function statusDotAndLabel(entry?: ContentEntry): {
  label: 'Published' | 'Modified' | 'Draft';
  dotClass: string;
  pillClass: string;
} {
  if (!entry || entry.status !== 'published') {
    return {
      label: 'Draft',
      dotClass: 'bg-sky-500',
      pillClass: 'bg-sky-50 text-sky-700 border-sky-200',
    };
  }
  if (entry.draft_version > entry.published_version) {
    return {
      label: 'Modified',
      dotClass: 'bg-violet-500',
      pillClass: 'bg-violet-50 text-violet-700 border-violet-200',
    };
  }
  return {
    label: 'Published',
    dotClass: 'bg-emerald-500',
    pillClass: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  };
}

export const RelationEntryPicker: React.FC<RelationEntryPickerProps> = ({
  contentTypeUid,
  multiple,
  value,
  onChange,
  disabled,
}) => {
  const searchInputId = React.useId();
  const [query, setQuery] = useState('');
  const [entries, setEntries] = useState<ContentEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState(0);
  const [draggedId, setDraggedId] = useState<string | null>(null);
  const [selectedEntries, setSelectedEntries] = useState<Record<string, ContentEntry>>({});
  const selected = useMemo(() => new Set(value), [value]);

  useEffect(() => {
    if (!contentTypeUid) return;
    let cancelled = false;
    const missing = value.filter((id) => !selectedEntries[id]);
    if (missing.length === 0) return;
    Promise.all(
      missing.map((id) => entriesApi.get(contentTypeUid, id).catch(() => null)),
    ).then((loaded) => {
      if (cancelled) return;
      const next: Record<string, ContentEntry> = {};
      loaded.forEach((entry) => {
        if (entry) next[entry.id] = entry;
      });
      if (Object.keys(next).length > 0) {
        setSelectedEntries((current) => ({ ...current, ...next }));
      }
    });
    return () => {
      cancelled = true;
    };
  }, [contentTypeUid, value, selectedEntries]);

  useEffect(() => {
    if (!contentTypeUid) {
      setEntries([]);
      setTotal(0);
      return;
    }
    const timer = window.setTimeout(() => {
      setLoading(true);
      setError(null);
      entriesApi
        .list(contentTypeUid, {
          limit: 20,
          offset,
          preview: true,
          search: query || undefined,
        })
        .then((response) => {
          setEntries(response.data || []);
          setTotal(response.meta?.total || 0);
        })
        .catch((reason: unknown) =>
          setError(
            reason instanceof Error
              ? reason.message
              : 'Failed to load related entries',
          ),
        )
        .finally(() => setLoading(false));
    }, 200);
    return () => window.clearTimeout(timer);
  }, [contentTypeUid, offset, query]);

  const choose = (entry: ContentEntry) => {
    setSelectedEntries((prev) => ({ ...prev, [entry.id]: entry }));
    if (selected.has(entry.id)) {
      onChange(value.filter((entryId) => entryId !== entry.id));
    } else if (multiple) {
      onChange([...value, entry.id]);
    } else {
      onChange([entry.id]);
    }
  };

  const removeId = (id: string) => {
    onChange(value.filter((entryId) => entryId !== id));
  };

  const moveByIndex = (from: number, to: number) => {
    if (from < 0 || to < 0 || from >= value.length || to >= value.length) return;
    const next = [...value];
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    onChange(next);
  };

  const reorderById = (targetId: string) => {
    if (!draggedId || draggedId === targetId) return;
    const from = value.indexOf(draggedId);
    const to = value.indexOf(targetId);
    moveByIndex(from, to);
  };

  const resolveEntry = (id: string) =>
    selectedEntries[id] || entries.find((item) => item.id === id);

  return (
    <div className="space-y-2.5">
      {/* Selected Relations Summary / Sortable Stack */}
      {value.length > 0 &&
        (multiple ? (
          <div className="space-y-1.5 rounded-xl border border-slate-200 bg-slate-50/70 p-2.5">
            <div className="flex items-center justify-between px-1 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
              <span>Selected ({value.length}) • Drag or reorder</span>
            </div>
            {value.map((id, index) => {
              const entry = resolveEntry(id);
              const label = entry ? labelFor(entry) : id;
              const status = statusDotAndLabel(entry);
              return (
                <div
                  key={id}
                  draggable={!disabled}
                  onDragStart={() => setDraggedId(id)}
                  onDragEnd={() => setDraggedId(null)}
                  onDragOver={(event) => event.preventDefault()}
                  onDrop={() => {
                    reorderById(id);
                    setDraggedId(null);
                  }}
                  className={`flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-2.5 py-2 text-xs text-slate-800 shadow-2xs ${
                    draggedId === id ? 'opacity-50' : ''
                  }`}
                >
                  <GripVertical
                    className="h-4 w-4 shrink-0 cursor-grab text-slate-300"
                    aria-hidden="true"
                  />
                  <span
                    className={`h-2 w-2 shrink-0 rounded-full ${status.dotClass}`}
                    title={status.label}
                  />
                  <span className="min-w-0 flex-1 truncate font-medium">
                    {label}
                  </span>
                  <span
                    className={`hidden sm:inline-flex rounded-full border px-1.5 py-0 text-[10px] font-semibold ${status.pillClass}`}
                  >
                    {status.label}
                  </span>
                  <div className="flex items-center gap-0.5">
                    <button
                      type="button"
                      disabled={disabled || index === 0}
                      onClick={() => moveByIndex(index, index - 1)}
                      aria-label={`Move ${label} up`}
                      className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700 disabled:opacity-30 cursor-pointer"
                    >
                      <ArrowUp className="h-3 w-3" />
                    </button>
                    <button
                      type="button"
                      disabled={disabled || index === value.length - 1}
                      onClick={() => moveByIndex(index, index + 1)}
                      aria-label={`Move ${label} down`}
                      className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700 disabled:opacity-30 cursor-pointer"
                    >
                      <ArrowDown className="h-3 w-3" />
                    </button>
                    <button
                      type="button"
                      disabled={disabled}
                      onClick={() => removeId(id)}
                      aria-label={`Remove ${id}`}
                      className="rounded p-1 text-slate-400 hover:bg-rose-50 hover:text-rose-600 cursor-pointer"
                    >
                      <X className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          (() => {
            const entry = resolveEntry(value[0]);
            const label = entry ? labelFor(entry) : value[0];
            const status = statusDotAndLabel(entry);
            return (
              <div className="flex items-center justify-between gap-2 rounded-lg border border-sky-200 bg-sky-50/70 px-3 py-2 text-xs">
                <div className="flex min-w-0 items-center gap-2">
                  <span className={`h-2 w-2 shrink-0 rounded-full ${status.dotClass}`} />
                  <span className="truncate font-semibold text-slate-900">
                    {label}
                  </span>
                  <span
                    className={`rounded-full border px-1.5 py-0 text-[10px] font-semibold ${status.pillClass}`}
                  >
                    {status.label}
                  </span>
                </div>
                <button
                  type="button"
                  disabled={disabled}
                  onClick={() => removeId(value[0])}
                  aria-label={`Remove ${value[0]}`}
                  className="rounded p-1 text-slate-400 hover:bg-rose-50 hover:text-rose-600 cursor-pointer"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
            );
          })()
        ))}

      {/* Searchable Combobox Selector */}
      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white">
        <div className="relative border-b border-slate-100 bg-slate-50/50">
          <Search className="pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
          <input
            id={searchInputId}
            name={`relation-search-${contentTypeUid || 'entries'}`}
            type="search"
            disabled={disabled || !contentTypeUid}
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setOffset(0);
            }}
            placeholder={`Search ${contentTypeUid || 'entries'} to connect...`}
            aria-expanded="true"
            aria-controls="relation-entry-options"
            className="w-full bg-transparent py-2 pr-8 pl-8 text-xs text-slate-900 placeholder:text-slate-400 focus:outline-none"
          />
          {query && (
            <button
              type="button"
              onClick={() => {
                setQuery('');
                setOffset(0);
              }}
              aria-label="Clear search"
              className="absolute top-1/2 right-2.5 -translate-y-1/2 text-slate-400 hover:text-slate-600 cursor-pointer"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>

        {loading && (
          <p className="px-3 py-2 text-xs text-slate-400">Loading entries…</p>
        )}
        {error && (
          <p role="alert" className="px-3 py-2 text-xs text-rose-600">
            {error}
          </p>
        )}
        {!loading && !error && entries.length === 0 && (
          <p className="px-3 py-3 text-center text-xs text-slate-500">
            No matching {contentTypeUid || 'target'} entries found.
          </p>
        )}

        <div
          id="relation-entry-options"
          role="listbox"
          aria-label="Related entries"
          className="max-h-44 divide-y divide-slate-100 overflow-y-auto"
        >
          {entries.map((entry) => {
            const label = labelFor(entry);
            const isSelected = selected.has(entry.id);
            const status = statusDotAndLabel(entry);
            return (
              <button
                key={entry.id}
                type="button"
                role="option"
                aria-label={label}
                aria-selected={isSelected}
                disabled={disabled || (multiple && isSelected)}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => choose(entry)}
                className={`flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-xs transition-colors ${
                  isSelected
                    ? 'bg-sky-50/70 text-sky-800 font-semibold'
                    : 'text-slate-700 hover:bg-slate-50 cursor-pointer'
                } disabled:opacity-60`}
              >
                <div className="flex min-w-0 items-center gap-2">
                  <span
                    className={`h-2 w-2 shrink-0 rounded-full ${status.dotClass}`}
                    aria-hidden="true"
                  />
                  <span className="truncate">{label}</span>
                </div>
                <div className="flex shrink-0 items-center gap-2" aria-hidden="true">
                  <span className="text-[10px] text-slate-400">{status.label}</span>
                  {isSelected && <Check className="h-3.5 w-3.5 text-sky-600" />}
                </div>
              </button>
            );
          })}
        </div>

        {total > 20 && (
          <div className="flex items-center justify-between border-t border-slate-100 bg-slate-50/70 px-3 py-1.5 text-[11px] text-slate-600">
            <button
              type="button"
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - 20))}
              className="inline-flex items-center gap-0.5 font-medium hover:text-sky-700 disabled:opacity-40 cursor-pointer"
            >
              <ChevronLeft className="h-3.5 w-3.5" />
              Prev
            </button>
            <span>
              {offset + 1}–{Math.min(offset + 20, total)} of {total}
            </span>
            <button
              type="button"
              disabled={offset + 20 >= total}
              onClick={() => setOffset(offset + 20)}
              className="inline-flex items-center gap-0.5 font-medium hover:text-sky-700 disabled:opacity-40 cursor-pointer"
            >
              Next
              <ChevronRight className="h-3.5 w-3.5" />
            </button>
          </div>
        )}
      </div>
    </div>
  );
};
