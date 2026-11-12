import { useMemo, useRef, useState } from 'react';
import {
  AlertCircle,
  ArrowDown,
  ArrowUp,
  Blocks,
  ChevronDown,
  ChevronRight,
  Copy,
  FolderOpen,
  GripVertical,
  Layers,
  Plus,
  Search,
  Trash2,
  X,
} from 'lucide-react';
import type { ContentType } from '../../types';
import { SchemaFields } from './FieldEditor';
import { configFor, isRecord, moveItem } from './contracts';
import type { FieldEditorProps } from './contracts';

export const MAX_COMPONENT_DEPTH = 8;

function componentCategoryOf(type: ContentType): string {
  const fromSettings =
    typeof type.settings?.category === 'string' ? type.settings.category.trim() : '';
  if (fromSettings) return fromSettings;
  const dot = type.uid.indexOf('.');
  return dot > 0 ? type.uid.slice(0, dot) : 'shared';
}

function summarizeComponentItem(
  item: unknown,
  definition?: ContentType,
): string | null {
  if (!isRecord(item) || !definition?.fields?.length) return null;
  for (const field of definition.fields) {
    const val = item[field.field_key];
    if (typeof val === 'string' && val.trim()) {
      const stripped = val.replace(/<[^>]+>/g, '').trim();
      if (stripped) return stripped.slice(0, 60);
    }
    if (typeof val === 'number' || typeof val === 'boolean') {
      return `${field.name}: ${String(val)}`;
    }
  }
  return null;
}

let nextComponentItemKey = 1;

export function ComponentEditor(props: FieldEditorProps) {
  const {
    field,
    value,
    onChange,
    definitions,
    disabled,
    depth = 0,
    path = field.field_key,
    errors,
    onValidityChange,
  } = props;
  const config = configFor(field);
  const dynamic = String(field.field_type) === 'dynamic_zone';
  const multiple =
    dynamic ||
    String(field.field_type) === 'repeatable_component' ||
    Boolean(config.repeatable);

  const available = useMemo(
    () => definitions.filter((definition) => String(definition.kind) === 'component'),
    [definitions],
  );
  const allowed = useMemo(
    () => (dynamic ? config.components || [] : config.component ? [config.component] : []),
    [dynamic, config.components, config.component],
  );

  const allowedDefinitions = useMemo(
    () => available.filter((def) => allowed.includes(def.uid)),
    [available, allowed],
  );

  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerSearch, setPickerSearch] = useState('');
  const [collapsed, setCollapsed] = useState<Set<number>>(new Set());
  const [itemKeys, setItemKeys] = useState<number[]>([]);
  const drag = useRef<number | null>(null);

  const empty = value == null || value === '';
  const malformed = !empty && (multiple ? !Array.isArray(value) : !isRecord(value));
  const items: unknown[] = empty
    ? []
    : multiple
      ? Array.isArray(value)
        ? value
        : []
      : [value];

  const resolvedKeys = items.map((_, index) => itemKeys[index] ?? -(index + 1));

  const groupedAllowed = useMemo(() => {
    const q = pickerSearch.trim().toLowerCase();
    const groups = new Map<string, ContentType[]>();
    for (const def of allowedDefinitions) {
      if (
        q &&
        !def.name.toLowerCase().includes(q) &&
        !def.uid.toLowerCase().includes(q)
      ) {
        continue;
      }
      const cat = componentCategoryOf(def);
      const list = groups.get(cat) || [];
      list.push(def);
      groups.set(cat, list);
    }
    return Array.from(groups.entries()).sort(([a], [b]) => a.localeCompare(b));
  }, [allowedDefinitions, pickerSearch]);

  if (depth >= MAX_COMPONENT_DEPTH) {
    return (
      <p role="alert" className="text-sm text-amber-700">
        Maximum component depth ({MAX_COMPONENT_DEPTH}) reached. The stored value is preserved.
      </p>
    );
  }

  if (malformed) {
    return (
      <div className="space-y-2 rounded-lg border border-amber-200 bg-amber-50 p-3">
        <p role="alert" className="text-sm text-amber-800">
          Legacy {typeof value === 'string' ? 'string' : 'structured'} value preserved. Repair this value explicitly before editing components.
        </p>
        <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded bg-white p-2 text-xs text-slate-700">
          {typeof value === 'string' ? value : JSON.stringify(value, null, 2)}
        </pre>
      </div>
    );
  }

  const emit = (next: unknown[]) => onChange(multiple ? next : next[0] ?? null);

  const addComponentInstance = (targetUid?: string) => {
    const uid = dynamic ? targetUid : config.component;
    const definition = available.find((candidate) => candidate.uid === uid);
    if (!definition || !allowed.includes(definition.uid) || disabled) return;
    const initial: Record<string, unknown> = {};
    for (const child of definition.fields || []) {
      const settings = configFor(child);
      if (settings.default !== undefined && !settings.read_only && !settings.hidden) {
        initial[child.field_key] = structuredClone(settings.default);
      }
    }
    if (dynamic) initial.__component = definition.uid;
    setItemKeys((prev) => [...items.map((_, i) => prev[i] ?? -(i + 1)), nextComponentItemKey++]);
    emit([...items, initial]);
    setPickerOpen(false);
    setPickerSearch('');
  };

  const move = (from: number, to: number) => {
    if (disabled || from === to) return;
    setItemKeys((prev) => moveItem(items.map((_, i) => prev[i] ?? -(i + 1)), from, to));
    emit(moveItem(items, from, to));
  };

  const atMaximum = config.max_items !== undefined && items.length >= config.max_items;
  const atMinimum = items.length <= Math.max(config.min_items || 0, config.required ? 1 : 0);

  return (
    <div
      className="space-y-3"
      onDragOver={(event) => event.stopPropagation()}
      onDrop={(event) => event.stopPropagation()}
    >
      {/* Empty State Banner */}
      {items.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-slate-300 bg-slate-50/70 px-6 py-7 text-center">
          <div className="mb-2 flex h-9 w-9 items-center justify-center rounded-xl bg-white text-slate-400 shadow-2xs ring-1 ring-slate-200">
            {dynamic ? <Blocks className="h-4 w-4" /> : <Layers className="h-4 w-4" />}
          </div>
          <p className="text-xs font-medium text-slate-600">
            No entry yet. Click below to add one.
          </p>
        </div>
      ) : (
        <div className="space-y-2.5">
          {items.map((item, index) => {
            const key = resolvedKeys[index];
            const uid =
              dynamic && isRecord(item) ? String(item.__component || '') : config.component;
            const definition = available.find((candidate) => candidate.uid === uid);
            const permitted = Boolean(uid && allowed.includes(uid));
            const itemPath = multiple ? `${path}[${index}]` : path;
            const closed = collapsed.has(key);
            const subtitle = summarizeComponentItem(item, definition);
            const itemHasError = Boolean(
              errors &&
                Object.entries(errors).some(
                  ([errPath, errMsg]) =>
                    Boolean(errMsg) &&
                    (errPath === itemPath || errPath.startsWith(`${itemPath}.`)),
                ),
            );

            return (
              <div
                key={key}
                className={`overflow-hidden rounded-xl border bg-white shadow-2xs transition-colors ${
                  itemHasError ? 'border-rose-300' : 'border-slate-200'
                }`}
                onDragOver={(event) => {
                  event.stopPropagation();
                  if (drag.current !== null && !disabled) event.preventDefault();
                }}
                onDrop={(event) => {
                  event.stopPropagation();
                  event.preventDefault();
                  if (drag.current !== null) move(drag.current, index);
                  drag.current = null;
                }}
              >
                {/* Accordion Header */}
                <div
                  className={`flex flex-wrap items-center gap-2 border-b px-3.5 py-2.5 ${
                    closed ? 'border-transparent' : 'border-slate-200 bg-slate-50/70'
                  }`}
                >
                  {multiple && (
                    <button
                      type="button"
                      disabled={disabled}
                      draggable={!disabled}
                      aria-label={`Drag item ${index + 1}`}
                      onDragStart={(event) => {
                        event.stopPropagation();
                        drag.current = index;
                        event.dataTransfer.setData('text/plain', String(key));
                        event.dataTransfer.effectAllowed = 'move';
                      }}
                      onDragEnd={(event) => {
                        event.stopPropagation();
                        drag.current = null;
                      }}
                      className="rounded p-1 text-slate-300 hover:text-slate-600 cursor-grab disabled:cursor-not-allowed"
                    >
                      <GripVertical className="h-4 w-4" />
                    </button>
                  )}

                  <button
                    type="button"
                    aria-expanded={!closed}
                    onClick={() =>
                      setCollapsed((current) => {
                        const next = new Set(current);
                        if (next.has(key)) next.delete(key);
                        else next.add(key);
                        return next;
                      })
                    }
                    className="flex min-w-0 flex-1 items-center gap-2 text-left cursor-pointer"
                  >
                    {closed ? (
                      <ChevronRight className="h-4 w-4 shrink-0 text-slate-400" />
                    ) : (
                      <ChevronDown className="h-4 w-4 shrink-0 text-slate-500" />
                    )}
                    <span className="truncate text-sm font-semibold text-slate-800">
                      {definition?.name || uid || 'Unknown component'}
                    </span>
                    {multiple && (
                      <span className="rounded bg-slate-200/70 px-1.5 py-0 text-[10px] font-bold text-slate-600">
                        #{index + 1}
                      </span>
                    )}
                    {dynamic && uid && (
                      <code className="hidden rounded bg-indigo-50 px-1.5 py-0 text-[10px] font-mono text-indigo-700 sm:inline">
                        {uid}
                      </code>
                    )}
                    {subtitle && (
                      <span className="truncate text-xs text-slate-400">
                        — {subtitle}
                      </span>
                    )}
                    {itemHasError && (
                      <span className="inline-flex items-center gap-1 rounded-full bg-rose-100 px-2 py-0.5 text-[10px] font-semibold text-rose-700">
                        <AlertCircle className="h-3 w-3" />
                        Invalid
                      </span>
                    )}
                  </button>

                  <div className="flex items-center gap-1">
                    {multiple && (
                      <>
                        <button
                          type="button"
                          disabled={disabled || index === 0}
                          aria-label={`Move item ${index + 1} up`}
                          onClick={() => move(index, index - 1)}
                          className="rounded border border-slate-200 bg-white p-1 text-slate-500 hover:bg-slate-100 disabled:opacity-30 cursor-pointer"
                        >
                          <ArrowUp className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          disabled={disabled || index === items.length - 1}
                          aria-label={`Move item ${index + 1} down`}
                          onClick={() => move(index, index + 1)}
                          className="rounded border border-slate-200 bg-white p-1 text-slate-500 hover:bg-slate-100 disabled:opacity-30 cursor-pointer"
                        >
                          <ArrowDown className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          disabled={disabled || atMaximum}
                          aria-label={`Duplicate item ${index + 1}`}
                          title="Duplicate entry"
                          onClick={() => {
                            setItemKeys((prev) => {
                              const nextKeys = items.map((_, i) => prev[i] ?? -(i + 1));
                              nextKeys.splice(index + 1, 0, nextComponentItemKey++);
                              return nextKeys;
                            });
                            const next = [...items];
                            next.splice(index + 1, 0, structuredClone(item));
                            emit(next);
                          }}
                          className="rounded border border-slate-200 bg-white p-1 text-slate-500 hover:bg-slate-100 disabled:opacity-30 cursor-pointer"
                        >
                          <Copy className="h-3.5 w-3.5" />
                        </button>
                      </>
                    )}
                    <button
                      type="button"
                      disabled={disabled || atMinimum}
                      aria-label={`Remove item ${index + 1}`}
                      title="Delete entry"
                      onClick={() => {
                        setItemKeys((prev) =>
                          items
                            .map((_, i) => prev[i] ?? -(i + 1))
                            .filter((_, current) => current !== index),
                        );
                        emit(items.filter((_, current) => current !== index));
                      }}
                      className="rounded border border-slate-200 bg-white p-1 text-slate-400 hover:border-rose-200 hover:bg-rose-50 hover:text-rose-600 disabled:opacity-30 cursor-pointer"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>

                {/* Mounted Accordion Body */}
                <div hidden={closed} className="p-4">
                  {definition && permitted && isRecord(item) ? (
                    <SchemaFields
                      fields={definition.fields || []}
                      data={item}
                      onDataChange={(nextItem) =>
                        emit(
                          items.map((current, currentIndex) =>
                            currentIndex === index
                              ? dynamic
                                ? { ...nextItem, __component: uid }
                                : nextItem
                              : current,
                          ),
                        )
                      }
                      definitions={definitions}
                      disabled={disabled}
                      depth={depth + 1}
                      path={itemPath}
                      errors={errors}
                      onValidityChange={onValidityChange}
                    />
                  ) : (
                    <p role="alert" className="text-sm text-amber-700">
                      {!isRecord(item)
                        ? 'Legacy item is not an object.'
                        : !definition
                          ? `Component definition ${uid || '(missing UID)'} is unavailable.`
                          : 'This component is no longer allowed.'}{' '}
                      The stored value is preserved.
                    </p>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Add Entry / Dynamic Zone Picker Controls */}
      {(multiple || items.length === 0) && (
        <div className="space-y-2.5">
          {dynamic ? (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <button
                  type="button"
                  disabled={disabled || atMaximum || allowedDefinitions.length === 0}
                  onClick={() => setPickerOpen((open) => !open)}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-sky-200 bg-sky-50 px-3.5 py-2 text-xs font-semibold text-sky-700 transition-colors hover:bg-sky-100 disabled:opacity-50 cursor-pointer"
                >
                  <Plus className="h-3.5 w-3.5" />
                  Add a component to {field.name || field.field_key}
                </button>
                {atMaximum && (
                  <span className="text-xs text-slate-500">
                    Maximum {config.max_items} items reached
                  </span>
                )}
              </div>

              {pickerOpen && !disabled && !atMaximum && (
                <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-lg space-y-3">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-xs font-bold uppercase tracking-wider text-slate-500">
                      Pick a component to add
                    </span>
                    <button
                      type="button"
                      onClick={() => setPickerOpen(false)}
                      aria-label="Close component picker"
                      className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700 cursor-pointer"
                    >
                      <X className="h-4 w-4" />
                    </button>
                  </div>

                  {allowedDefinitions.length >= 4 && (
                    <div className="relative">
                      <Search className="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
                      <input
                        name="dynamicZoneComponentSearch"
                        type="search"
                        value={pickerSearch}
                        onChange={(e) => setPickerSearch(e.target.value)}
                        placeholder="Filter allowed components..."
                        className="w-full rounded-lg border border-slate-200 bg-slate-50 py-1.5 pr-3 pl-8 text-xs focus:border-sky-500 focus:bg-white focus:outline-none"
                      />
                    </div>
                  )}

                  {groupedAllowed.length === 0 ? (
                    <p className="py-3 text-center text-xs text-slate-500">
                      No matching components configured for this Dynamic Zone.
                    </p>
                  ) : (
                    <div className="space-y-3">
                      {groupedAllowed.map(([category, defs]) => (
                        <div key={category} className="space-y-1.5">
                          <div className="flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">
                            <FolderOpen className="h-3 w-3" />
                            <span>{category}</span>
                          </div>
                          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                            {defs.map((def) => (
                              <button
                                key={def.uid}
                                type="button"
                                onClick={() => addComponentInstance(def.uid)}
                                className="flex items-center gap-2.5 rounded-lg border border-slate-200 bg-slate-50/50 px-3 py-2.5 text-left transition-colors hover:border-sky-400 hover:bg-sky-50 cursor-pointer"
                              >
                                <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-white text-indigo-600 ring-1 ring-slate-200">
                                  <Layers className="h-3.5 w-3.5" />
                                </span>
                                <div className="min-w-0">
                                  <div className="truncate text-xs font-semibold text-slate-900">
                                    {def.name}
                                  </div>
                                  <div className="truncate text-[10px] font-mono text-slate-400">
                                    {def.fields?.length || 0} fields
                                  </div>
                                </div>
                              </button>
                            ))}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </>
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <button
                type="button"
                disabled={
                  disabled ||
                  atMaximum ||
                  !available.some((definition) => definition.uid === config.component)
                }
                onClick={() => addComponentInstance()}
                className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-3.5 py-2 text-xs font-semibold text-sky-700 transition-colors hover:border-sky-300 hover:bg-sky-50 disabled:opacity-50 cursor-pointer"
              >
                <Plus className="h-3.5 w-3.5" />
                Add an entry
              </button>
              {atMaximum && (
                <span className="text-xs text-slate-500">
                  Maximum {config.max_items} items
                </span>
              )}
            </div>
          )}
        </div>
      )}

      {!dynamic && !available.some((definition) => definition.uid === config.component) && (
        <p role="alert" className="text-xs text-amber-700">
          Component definition {config.component || '(not configured)'} is unavailable.
        </p>
      )}
    </div>
  );
}
