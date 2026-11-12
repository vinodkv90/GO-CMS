import React, { useId, useMemo, useRef, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  Check,
  FolderOpen,
  Layers,
  Plus,
  Repeat,
  Search,
  Trash2,
} from 'lucide-react';
import type { ContentField, ContentType, FieldType, ValidationRules } from '../../types';
import {
  eligibleUIDSources,
  FIELD_REGISTRY,
  getFieldSettings,
  initialConfiguration,
} from '../../fields/registry';
import type { FieldCapabilityMap, SettingDefinition } from '../../fields/registry';
import { Button } from '../ui/Button';
import { Switch } from '../ui/Switch';
import { RelationBuilder } from './RelationBuilder';

const controlClass =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-sky-500 focus:outline-none focus:ring-2 focus:ring-sky-500/20 disabled:bg-slate-100';

const ALLOWED_MEDIA_TYPES: Array<{ id: string; label: string; hint: string }> = [
  { id: 'image', label: 'Images', hint: 'JPEG, PNG, GIF, WebP, SVG' },
  { id: 'video', label: 'Videos', hint: 'MP4, WebM, HLS packages' },
  { id: 'audio', label: 'Audios', hint: 'MP3, WAV, OGG' },
  { id: 'file', label: 'Files', hint: 'PDF, documents, archives' },
];

const SAFE_URL_PROTOCOLS = ['https', 'http', 'mailto', 'tel', 'ftp', 'ftps'] as const;

const BLOCK_TYPE_OPTIONS: Array<{ id: string; label: string }> = [
  { id: 'paragraph', label: 'Paragraph' },
  { id: 'heading', label: 'Heading' },
  { id: 'bulletList', label: 'Bullet List' },
  { id: 'orderedList', label: 'Ordered List' },
  { id: 'listItem', label: 'List Item' },
  { id: 'blockquote', label: 'Blockquote' },
  { id: 'codeBlock', label: 'Code Block' },
  { id: 'image', label: 'Image' },
  { id: 'text', label: 'Text Node' },
];

const COMMON_TIMEZONES = [
  'UTC',
  'Asia/Kolkata',
  'America/New_York',
  'America/Los_Angeles',
  'Europe/London',
  'Europe/Berlin',
  'Asia/Tokyo',
  'Australia/Sydney',
];

function componentCategoryOf(type: ContentType): string {
  const fromSettings =
    typeof type.settings?.category === 'string' ? type.settings.category.trim() : '';
  if (fromSettings) return fromSettings;
  const dot = type.uid.indexOf('.');
  return dot > 0 ? type.uid.slice(0, dot) : 'shared';
}

function ControlLabel({
  label,
  help,
  children,
}: {
  label: string;
  help?: string;
  children: (id: string) => React.ReactNode;
}) {
  const id = useId();
  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="block text-xs font-semibold text-slate-700">
        {label}
      </label>
      {children(id)}
      {help && <p className="text-xs text-slate-500">{help}</p>}
    </div>
  );
}

function StringRows({
  label,
  values,
  onChange,
  itemLabel = 'option',
}: {
  label: string;
  values: string[];
  onChange: (values: string[]) => void;
  itemLabel?: string;
}) {
  const sequence = useRef(values.length);
  const [ids, setIds] = useState(() => values.map((_, index) => index));

  const move = (index: number, delta: number) => {
    const next = [...values];
    const nextIds = [...ids];
    [next[index], next[index + delta]] = [next[index + delta], next[index]];
    [nextIds[index], nextIds[index + delta]] = [nextIds[index + delta], nextIds[index]];
    setIds(nextIds);
    onChange(next);
  };

  return (
    <fieldset className="space-y-2">
      <legend className="mb-1 text-xs font-semibold text-slate-700">{label}</legend>
      <div className="space-y-1.5">
        {values.map((value, index) => (
          <div key={ids[index] ?? index} className="flex items-center gap-1.5">
            <input
              name={`${label}-${index}`}
              aria-label={`${label} ${index + 1}`}
              placeholder={`Enter ${itemLabel}...`}
              className={`${controlClass} min-w-0 flex-1`}
              value={value}
              onChange={(event) =>
                onChange(
                  values.map((item, i) => (i === index ? event.target.value : item)),
                )
              }
            />
            <button
              type="button"
              disabled={index === 0}
              onClick={() => move(index, -1)}
              aria-label={`Move ${itemLabel} ${index + 1} up`}
              className="rounded-lg border border-slate-200 p-2 text-slate-500 hover:bg-slate-100 disabled:opacity-30 cursor-pointer"
            >
              <ArrowUp className="h-3.5 w-3.5" />
            </button>
            <button
              type="button"
              disabled={index === values.length - 1}
              onClick={() => move(index, 1)}
              aria-label={`Move ${itemLabel} ${index + 1} down`}
              className="rounded-lg border border-slate-200 p-2 text-slate-500 hover:bg-slate-100 disabled:opacity-30 cursor-pointer"
            >
              <ArrowDown className="h-3.5 w-3.5" />
            </button>
            <button
              type="button"
              onClick={() => {
                setIds(ids.filter((_, i) => i !== index));
                onChange(values.filter((_, i) => i !== index));
              }}
              aria-label={`Remove ${itemLabel} ${index + 1}`}
              className="rounded-lg border border-slate-200 p-2 text-slate-400 hover:bg-rose-50 hover:text-rose-600 cursor-pointer"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          </div>
        ))}
      </div>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => {
          setIds([...ids, sequence.current++]);
          onChange([...values, '']);
        }}
      >
        <Plus className="h-3.5 w-3.5" />
        Add {itemLabel}
      </Button>
    </fieldset>
  );
}

function JSONSetting({
  label,
  value,
  onChange,
  onError,
  objectOnly = false,
}: {
  label: string;
  value: unknown;
  onChange: (value: unknown) => void;
  onError: (error: string | null) => void;
  objectOnly?: boolean;
}) {
  const [raw, setRaw] = useState(() =>
    value === undefined ? '' : JSON.stringify(value, null, 2),
  );
  const [error, setError] = useState<string | null>(null);

  return (
    <ControlLabel label={label}>
      {(id) => (
        <>
          <textarea
            id={id}
            rows={4}
            value={raw}
            spellCheck={false}
            aria-invalid={Boolean(error)}
            aria-describedby={error ? `${id}-error` : undefined}
            className={`${controlClass} font-mono text-xs`}
            onChange={(event) => {
              const text = event.target.value;
              setRaw(text);
              try {
                const parsed: unknown = text.trim() ? JSON.parse(text) : undefined;
                if (
                  objectOnly &&
                  parsed !== undefined &&
                  (!parsed || typeof parsed !== 'object' || Array.isArray(parsed))
                ) {
                  throw new Error('Enter a JSON object.');
                }
                setError(null);
                onError(null);
                onChange(parsed);
              } catch (reason) {
                const message = reason instanceof Error ? reason.message : 'Invalid JSON';
                setError(message);
                onError(`${label}: ${message}`);
              }
            }}
          />
          {error && (
            <p id={`${id}-error`} role="alert" className="text-xs text-rose-700">
              {error}
            </p>
          )}
        </>
      )}
    </ControlLabel>
  );
}

function SwitchCard({
  label,
  help,
  checked,
  onChange,
}: {
  label: string;
  help?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  const id = useId();
  return (
    <div className="flex items-start justify-between gap-3 rounded-xl border border-slate-200 bg-white p-3.5">
      <div className="min-w-0">
        <label htmlFor={id} className="block text-sm font-semibold text-slate-800 cursor-pointer">
          {label}
        </label>
        {help && <p className="mt-0.5 text-xs text-slate-500">{help}</p>}
      </div>
      <Switch id={id} checked={checked} onChange={onChange} />
    </div>
  );
}

export interface FieldSettingsProps {
  field: ContentField;
  owner: ContentType;
  contentTypes: ContentType[];
  isEditing: boolean;
  activeTab: 'basic' | 'advanced';
  onFieldTypeChange: (nextType: FieldType, nextConfig: ValidationRules) => void;
  onChange: (config: ValidationRules) => void;
  onError: (key: string, error: string | null) => void;
  serverCapabilities?: FieldCapabilityMap;
}

export function FieldSettings({
  field,
  owner,
  contentTypes,
  isEditing,
  activeTab,
  onFieldTypeChange,
  onChange,
  onError,
  serverCapabilities,
}: FieldSettingsProps) {
  const [componentSearch, setComponentSearch] = useState('');
  const timezoneListId = useId();
  const config = field.configuration;

  const isCustomType = !(field.field_type in FIELD_REGISTRY);
  const definitions = useMemo<SettingDefinition[]>(
    () =>
      isCustomType
        ? []
        : getFieldSettings(field.field_type, config, serverCapabilities),
    [field.field_type, config, serverCapabilities, isCustomType],
  );
  const allowedKeys = useMemo(
    () => new Set(definitions.map((d) => d.key)),
    [definitions],
  );

  const update = (key: string, value: unknown) => {
    const next: ValidationRules = { ...config };
    if (value === undefined) {
      delete next[key];
    } else {
      next[key] = value;
    }
    onChange(next);
  };

  const components = useMemo(
    () =>
      contentTypes
        .filter((t) => t.kind === 'component' && t.uid !== owner.uid)
        .sort((a, b) => a.name.localeCompare(b.name)),
    [contentTypes, owner.uid],
  );

  const filteredComponents = useMemo(() => {
    const q = componentSearch.trim().toLowerCase();
    if (!q) return components;
    return components.filter(
      (c) =>
        c.name.toLowerCase().includes(q) ||
        c.uid.toLowerCase().includes(q) ||
        componentCategoryOf(c).toLowerCase().includes(q),
    );
  }, [components, componentSearch]);

  const groupedComponents = useMemo(() => {
    const groups = new Map<string, ContentType[]>();
    for (const c of filteredComponents) {
      const cat = componentCategoryOf(c);
      const list = groups.get(cat) || [];
      list.push(c);
      groups.set(cat, list);
    }
    return Array.from(groups.entries()).sort(([a], [b]) => a.localeCompare(b));
  }, [filteredComponents]);

  const switchSubtype = (nextType: FieldType, extraConfig?: Partial<ValidationRules>) => {
    const preservedCommon: ValidationRules = {
      ...initialConfiguration(nextType),
      layout_width: config.layout_width || 'full',
      description: config.description,
      placeholder: config.placeholder,
      group: config.group,
      required: config.required,
      unique: config.unique,
      private: config.private,
      read_only: config.read_only,
      hidden: config.hidden,
      error_message: config.error_message,
      component: config.component,
      ...extraConfig,
    };
    const nextCapKeys = new Set(
      getFieldSettings(nextType, preservedCommon, serverCapabilities).map((s) => s.key),
    );
    const cleaned: ValidationRules = {};
    for (const [k, v] of Object.entries(preservedCommon)) {
      if (v !== undefined && nextCapKeys.has(k)) {
        cleaned[k] = v;
      }
    }
    onError('default', null);
    onFieldTypeChange(nextType, cleaned);
  };

  // --- BASIC SETTINGS TAB ---
  if (activeTab === 'basic') {
    const isTextSubtype = field.field_type === 'short_text' || field.field_type === 'long_text';
    const isNumberSubtype = field.field_type === 'number' || field.field_type === 'decimal';
    const isDateSubtype =
      field.field_type === 'date' ||
      field.field_type === 'time' ||
      field.field_type === 'datetime';
    const isComponentType =
      field.field_type === 'component' || field.field_type === 'repeatable_component';
    const isRelationType =
      field.field_type === 'relation_single' || field.field_type === 'relation_multiple';

    return (
      <div className="space-y-5">
        {/* Subtype Selectors matching Strapi Step 2 Basic UX */}
        {!isEditing && isTextSubtype && (
          <fieldset className="space-y-2">
            <legend className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              Text Format
            </legend>
            <div className="grid gap-3 sm:grid-cols-2">
              {[
                {
                  type: 'short_text' as const,
                  title: 'Short text',
                  desc: 'Best for titles, names, and single-line values',
                },
                {
                  type: 'long_text' as const,
                  title: 'Long text',
                  desc: 'Best for descriptions, summaries, and paragraphs',
                },
              ].map((opt) => {
                const active = field.field_type === opt.type;
                return (
                  <button
                    key={opt.type}
                    type="button"
                    onClick={() => switchSubtype(opt.type)}
                    className={`rounded-xl border p-3.5 text-left transition-all cursor-pointer ${
                      active
                        ? 'border-sky-500 bg-sky-50 ring-1 ring-sky-200'
                        : 'border-slate-200 bg-white hover:border-slate-300'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-semibold text-slate-900">
                        {opt.title}
                      </span>
                      <span
                        className={`h-4 w-4 rounded-full border-4 ${
                          active ? 'border-sky-600 bg-white' : 'border-slate-300 bg-white'
                        }`}
                      />
                    </div>
                    <p className="mt-1 text-xs text-slate-500">{opt.desc}</p>
                  </button>
                );
              })}
            </div>
          </fieldset>
        )}

        {!isEditing && isNumberSubtype && (
          <fieldset className="space-y-2">
            <legend className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              Number Format
            </legend>
            <div className="grid gap-3 sm:grid-cols-2">
              {[
                {
                  type: 'number' as const,
                  title: 'Integer (number)',
                  desc: 'Whole numbers (e.g. 10, 42)',
                },
                {
                  type: 'decimal' as const,
                  title: 'Decimal (float)',
                  desc: 'Floating point values (e.g. 3.14)',
                },
              ].map((opt) => {
                const active = field.field_type === opt.type;
                return (
                  <button
                    key={opt.type}
                    type="button"
                    onClick={() => switchSubtype(opt.type)}
                    className={`rounded-xl border p-3.5 text-left transition-all cursor-pointer ${
                      active
                        ? 'border-sky-500 bg-sky-50 ring-1 ring-sky-200'
                        : 'border-slate-200 bg-white hover:border-slate-300'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-semibold text-slate-900">
                        {opt.title}
                      </span>
                      <span
                        className={`h-4 w-4 rounded-full border-4 ${
                          active ? 'border-sky-600 bg-white' : 'border-slate-300 bg-white'
                        }`}
                      />
                    </div>
                    <p className="mt-1 text-xs text-slate-500">{opt.desc}</p>
                  </button>
                );
              })}
            </div>
          </fieldset>
        )}

        {!isEditing && isDateSubtype && (
          <fieldset className="space-y-2">
            <legend className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              Temporal Format
            </legend>
            <div className="grid gap-3 sm:grid-cols-3">
              {[
                { type: 'date' as const, title: 'Date', desc: 'YYYY-MM-DD' },
                { type: 'datetime' as const, title: 'DateTime', desc: 'Date + time + UTC' },
                { type: 'time' as const, title: 'Time', desc: 'HH:mm:ss' },
              ].map((opt) => {
                const active = field.field_type === opt.type;
                return (
                  <button
                    key={opt.type}
                    type="button"
                    onClick={() => switchSubtype(opt.type)}
                    className={`rounded-xl border p-3 text-left transition-all cursor-pointer ${
                      active
                        ? 'border-sky-500 bg-sky-50 ring-1 ring-sky-200'
                        : 'border-slate-200 bg-white hover:border-slate-300'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-semibold text-slate-900">
                        {opt.title}
                      </span>
                      <span
                        className={`h-4 w-4 rounded-full border-4 ${
                          active ? 'border-sky-600 bg-white' : 'border-slate-300 bg-white'
                        }`}
                      />
                    </div>
                    <p className="mt-0.5 text-xs text-slate-500">{opt.desc}</p>
                  </button>
                );
              })}
            </div>
          </fieldset>
        )}

        {/* Media: Single vs Multiple radio cards */}
        {field.field_type === 'media' && (
          <fieldset className="space-y-2">
            <legend className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              Media Cardinality
            </legend>
            <div className="grid gap-3 sm:grid-cols-2">
              {[
                {
                  multiple: false,
                  title: 'Single media',
                  desc: 'Best for avatars, thumbnails, or cover images',
                },
                {
                  multiple: true,
                  title: 'Multiple media',
                  desc: 'Best for sliders, galleries, and document bundles',
                },
              ].map((opt) => {
                const active = Boolean(config.multiple) === opt.multiple;
                return (
                  <button
                    key={String(opt.multiple)}
                    type="button"
                    onClick={() => {
                      const next: ValidationRules = {
                        ...config,
                        multiple: opt.multiple,
                      };
                      if (!opt.multiple) {
                        delete next.min_items;
                        delete next.max_items;
                        delete next.preserve_order;
                      }
                      onChange(next);
                    }}
                    className={`rounded-xl border p-3.5 text-left transition-all cursor-pointer ${
                      active
                        ? 'border-sky-500 bg-sky-50 ring-1 ring-sky-200'
                        : 'border-slate-200 bg-white hover:border-slate-300'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-semibold text-slate-900">
                        {opt.title}
                      </span>
                      <span
                        className={`h-4 w-4 rounded-full border-4 ${
                          active ? 'border-sky-600 bg-white' : 'border-slate-300 bg-white'
                        }`}
                      />
                    </div>
                    <p className="mt-1 text-xs text-slate-500">{opt.desc}</p>
                  </button>
                );
              })}
            </div>
          </fieldset>
        )}

        {/* Relation: Visual Two-Box Builder */}
        {isRelationType && (
          <RelationBuilder
            owner={owner}
            contentTypes={contentTypes}
            field={field}
            isEditing={isEditing}
            onFieldTypeChange={(nextType, nextConfig) =>
              onFieldTypeChange(nextType, nextConfig)
            }
            onConfigChange={onChange}
          />
        )}

        {/* UID Attached Field */}
        {field.field_type === 'uid' && (
          <ControlLabel
            label="Attached field"
            help="Auto-generates slugs when typing in the linked text field."
          >
            {(id) => (
              <select
                id={id}
                className={controlClass}
                value={config.target_field || ''}
                onChange={(e) => update('target_field', e.target.value || undefined)}
              >
                <option value="">Manual entry (no automatic source)</option>
                {eligibleUIDSources(owner.fields || [], field.field_key).map((src) => (
                  <option key={src.field_key} value={src.field_key}>
                    {src.name} ({src.field_key})
                  </option>
                ))}
              </select>
            )}
          </ControlLabel>
        )}

        {/* Enumeration Values */}
        {field.field_type === 'enum' && (
          <div className="space-y-4 rounded-xl border border-slate-200 bg-slate-50/60 p-4">
            <StringRows
              label="Values (one per option)"
              itemLabel="option"
              values={Array.isArray(config.enum_values) ? config.enum_values : []}
              onChange={(values) => {
                const next: ValidationRules = { ...config, enum_values: values };
                if (
                  config.default !== undefined &&
                  !values.includes(String(config.default))
                ) {
                  delete next.default;
                }
                onChange(next);
              }}
            />

            <ControlLabel label="Default value">
              {(id) => {
                const validOptions = (config.enum_values || []).filter(
                  (v) => v.trim() !== '',
                );
                return (
                  <select
                    id={id}
                    className={controlClass}
                    value={config.default !== undefined ? String(config.default) : ''}
                    onChange={(e) =>
                      update('default', e.target.value || undefined)
                    }
                  >
                    <option value="">Choose here (no default)</option>
                    {validOptions.map((opt) => (
                      <option key={opt} value={opt}>
                        {opt}
                      </option>
                    ))}
                  </select>
                );
              }}
            </ControlLabel>
          </div>
        )}

        {/* Component Selector & Single/Repeatable Cards */}
        {isComponentType && (
          <div className="space-y-4">
            <fieldset className="space-y-2">
              <legend className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                Component Type
              </legend>
              <div className="grid gap-3 sm:grid-cols-2">
                {[
                  {
                    type: 'repeatable_component' as const,
                    repeatable: true,
                    icon: Repeat,
                    title: 'Repeatable component',
                    desc: 'Add an ordered array of reusable component items',
                  },
                  {
                    type: 'component' as const,
                    repeatable: false,
                    icon: Layers,
                    title: 'Single component',
                    desc: 'One embedded group of fields (e.g. SEO block)',
                  },
                ].map((opt) => {
                  const active = field.field_type === opt.type;
                  const disabled = isEditing && !active;
                  const Icon = opt.icon;
                  return (
                    <button
                      key={opt.type}
                      type="button"
                      disabled={disabled}
                      onClick={() => {
                        if (disabled) return;
                        switchSubtype(opt.type, {
                          repeatable: opt.repeatable,
                          preserve_order: opt.repeatable ? true : undefined,
                          min_items: opt.repeatable ? config.min_items : undefined,
                          max_items: opt.repeatable ? config.max_items : undefined,
                        });
                      }}
                      className={`rounded-xl border p-3.5 text-left transition-all ${
                        active
                          ? 'border-sky-500 bg-sky-50 ring-1 ring-sky-200'
                          : disabled
                            ? 'cursor-not-allowed border-slate-200 bg-slate-100 opacity-60'
                            : 'cursor-pointer border-slate-200 bg-white hover:border-slate-300'
                      }`}
                    >
                      <div className="flex items-center justify-between">
                        <span className="flex items-center gap-2 text-sm font-semibold text-slate-900">
                          <Icon className="h-4 w-4 text-sky-600" />
                          {opt.title}
                        </span>
                        <span
                          className={`h-4 w-4 rounded-full border-4 ${
                            active ? 'border-sky-600 bg-white' : 'border-slate-300 bg-white'
                          }`}
                        />
                      </div>
                      <p className="mt-1 text-xs text-slate-500">{opt.desc}</p>
                    </button>
                  );
                })}
              </div>
            </fieldset>

            <div className="space-y-2 rounded-xl border border-slate-200 bg-slate-50/70 p-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-xs font-semibold uppercase tracking-wide text-slate-600">
                  Select a component *
                </span>
                <div className="relative w-full sm:w-56">
                  <Search className="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
                  <input
                    id="ctb-component-search"
                    name="componentSearch"
                    type="search"
                    value={componentSearch}
                    onChange={(e) => setComponentSearch(e.target.value)}
                    placeholder="Filter components..."
                    className="w-full rounded-md border border-slate-300 bg-white py-1 pr-2.5 pl-8 text-xs focus:border-sky-500 focus:outline-none"
                  />
                </div>
              </div>

              {components.length === 0 ? (
                <p className="rounded-lg border border-dashed border-slate-300 bg-white p-4 text-center text-xs text-slate-500">
                  No components found yet. Close this dialog and click <strong>Create new component</strong> in the sidebar first.
                </p>
              ) : groupedComponents.length === 0 ? (
                <p className="p-3 text-center text-xs text-slate-500">
                  No components match &ldquo;{componentSearch}&rdquo;.
                </p>
              ) : (
                <div className="max-h-52 space-y-3 overflow-y-auto pr-1">
                  {groupedComponents.map(([category, list]) => (
                    <div key={category} className="space-y-1.5">
                      <div className="flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">
                        <FolderOpen className="h-3 w-3" />
                        <span>{category}</span>
                      </div>
                      <div className="grid gap-2 sm:grid-cols-2">
                        {list.map((comp) => {
                          const selected = config.component === comp.uid;
                          return (
                            <button
                              key={comp.uid}
                              type="button"
                              onClick={() => update('component', comp.uid)}
                              className={`flex items-center justify-between rounded-lg border px-3 py-2 text-left text-xs transition-colors cursor-pointer ${
                                selected
                                  ? 'border-sky-500 bg-sky-50 font-semibold text-sky-800'
                                  : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300'
                              }`}
                            >
                              <div className="min-w-0">
                                <div className="truncate font-semibold">{comp.name}</div>
                                <div className="truncate font-mono text-[11px] text-slate-400">
                                  {comp.uid}
                                </div>
                              </div>
                              {selected && <Check className="h-4 w-4 shrink-0 text-sky-600" />}
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}

        {/* Dynamic Zone Allowed Components Multi-Select */}
        {field.field_type === 'dynamic_zone' && (
          <div className="space-y-3 rounded-xl border border-slate-200 bg-slate-50/70 p-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <h4 className="text-xs font-semibold uppercase tracking-wide text-slate-600">
                  Pick allowed components *
                </h4>
                <p className="text-xs text-slate-500">
                  Editors can mix these components inside this Dynamic Zone.
                </p>
              </div>
              <div className="relative w-full sm:w-56">
                <Search className="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
                <input
                  id="ctb-dz-component-search"
                  name="dzComponentSearch"
                  type="search"
                  value={componentSearch}
                  onChange={(e) => setComponentSearch(e.target.value)}
                  placeholder="Filter components..."
                  className="w-full rounded-md border border-slate-300 bg-white py-1 pr-2.5 pl-8 text-xs focus:border-sky-500 focus:outline-none"
                />
              </div>
            </div>

            {components.length === 0 ? (
              <p className="rounded-lg border border-dashed border-slate-300 bg-white p-4 text-center text-xs text-slate-500">
                Create at least one Component definition in the left sidebar first.
              </p>
            ) : (
              <div className="max-h-56 space-y-3 overflow-y-auto pr-1">
                {groupedComponents.map(([category, list]) => {
                  const selected = Array.isArray(config.components) ? config.components : [];
                  return (
                    <div key={category} className="space-y-1.5">
                      <div className="flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">
                        <FolderOpen className="h-3 w-3" />
                        <span>{category}</span>
                      </div>
                      <div className="grid gap-2 sm:grid-cols-2">
                        {list.map((comp) => {
                          const checked = selected.includes(comp.uid);
                          return (
                            <button
                              key={comp.uid}
                              type="button"
                              onClick={() =>
                                update(
                                  'components',
                                  checked
                                    ? selected.filter((u) => u !== comp.uid)
                                    : [...selected, comp.uid],
                                )
                              }
                              className={`flex items-center justify-between rounded-lg border px-3 py-2 text-left text-xs transition-colors cursor-pointer ${
                                checked
                                  ? 'border-sky-500 bg-sky-50 text-sky-900'
                                  : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300'
                              }`}
                            >
                              <div className="min-w-0">
                                <div className="truncate font-semibold">{comp.name}</div>
                                <div className="truncate font-mono text-[11px] text-slate-400">
                                  {comp.uid}
                                </div>
                              </div>
                              <span
                                className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border ${
                                  checked
                                    ? 'border-sky-600 bg-sky-600 text-white'
                                    : 'border-slate-300 bg-white'
                                }`}
                              >
                                {checked && <Check className="h-3 w-3" />}
                              </span>
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}

            {(Array.isArray(config.components) ? config.components : [])
              .filter((uid) => !components.some((c) => c.uid === uid))
              .map((missingUid) => (
                <div
                  key={missingUid}
                  className="flex items-center justify-between rounded-lg border border-amber-200 bg-amber-50 px-3 py-1.5 text-xs text-amber-800"
                >
                  <span>Unavailable component: {missingUid}</span>
                  <button
                    type="button"
                    className="font-semibold underline cursor-pointer"
                    onClick={() =>
                      update(
                        'components',
                        (config.components || []).filter((u) => u !== missingUid),
                      )
                    }
                  >
                    Remove
                  </button>
                </div>
              ))}
          </div>
        )}

        {/* Common Basic Metadata */}
        <div className="grid gap-4 border-t border-slate-100 pt-4 sm:grid-cols-2">
          {(allowedKeys.has('description') || isCustomType) && (
            <ControlLabel
              label="Description / Help text"
              help="Displayed beneath this field inside Content Manager."
            >
              {(id) => (
                <input
                  id={id}
                  className={controlClass}
                  value={typeof config.description === 'string' ? config.description : ''}
                  onChange={(e) => update('description', e.target.value || undefined)}
                  placeholder="Optional guidance for editors"
                />
              )}
            </ControlLabel>
          )}

          {allowedKeys.has('placeholder') && (
            <ControlLabel label="Placeholder">
              {(id) => (
                <input
                  id={id}
                  className={controlClass}
                  value={typeof config.placeholder === 'string' ? config.placeholder : ''}
                  onChange={(e) => update('placeholder', e.target.value || undefined)}
                  placeholder="e.g. Enter article title..."
                />
              )}
            </ControlLabel>
          )}

          {(allowedKeys.has('layout_width') || isCustomType) && (
            <div className="space-y-1.5">
              <span className="block text-xs font-semibold text-slate-700">
                Editor column width
              </span>
              <div className="grid grid-cols-4 gap-1.5">
                {(
                  [
                    { id: 'full', label: 'Full' },
                    { id: 'two-thirds', label: '2/3' },
                    { id: 'half', label: '1/2' },
                    { id: 'third', label: '1/3' },
                  ] as const
                ).map((w) => {
                  const active = (config.layout_width || 'full') === w.id;
                  return (
                    <button
                      key={w.id}
                      type="button"
                      onClick={() => update('layout_width', w.id)}
                      className={`rounded-lg border py-2 text-xs font-semibold transition-colors cursor-pointer ${
                        active
                          ? 'border-sky-500 bg-sky-50 text-sky-700'
                          : 'border-slate-200 bg-white text-slate-600 hover:bg-slate-50'
                      }`}
                    >
                      {w.label}
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {(allowedKeys.has('group') || isCustomType) && (
            <ControlLabel label="Field group (optional section heading)">
              {(id) => (
                <input
                  id={id}
                  className={controlClass}
                  value={typeof config.group === 'string' ? config.group : ''}
                  onChange={(e) => update('group', e.target.value || undefined)}
                  placeholder="e.g. SEO Settings"
                />
              )}
            </ControlLabel>
          )}
        </div>
      </div>
    );
  }

  // --- ADVANCED SETTINGS TAB ---
  const showDefault = allowedKeys.has('default') && field.field_type !== 'enum';
  const kind = FIELD_REGISTRY[field.field_type]?.valueKind || 'string';
  const defaultEnabled = config.default !== undefined;

  return (
    <div className="space-y-6">
      {/* Default Value Card */}
      {showDefault && (
        <div className="space-y-3 rounded-xl border border-slate-200 bg-slate-50/70 p-4">
          <div className="flex items-center justify-between gap-2">
            <div>
              <h4 className="text-sm font-semibold text-slate-800">Default value</h4>
              <p className="text-xs text-slate-500">
                Pre-populate newly created entries when empty.
              </p>
            </div>
            <Switch
              label="Set default value"
              checked={defaultEnabled}
              onChange={(checked) => {
                onError('default', null);
                update(
                  'default',
                  checked
                    ? kind === 'boolean'
                      ? false
                      : kind === 'number'
                        ? 0
                        : kind === 'json'
                          ? field.field_type === 'blocks'
                            ? []
                            : {}
                          : ''
                    : undefined,
                );
              }}
            />
          </div>

          {defaultEnabled &&
            (kind === 'json' ? (
              <JSONSetting
                label="Default JSON value"
                value={config.default}
                onChange={(next) => update('default', next)}
                onError={(err) => onError('default', err)}
              />
            ) : kind === 'boolean' ? (
              <div className="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-3.5 py-2.5">
                <span className="text-xs font-semibold text-slate-700">
                  Initial switch state
                </span>
                <Switch
                  label="Default boolean state"
                  checked={Boolean(config.default)}
                  onLabel="TRUE"
                  offLabel="FALSE"
                  onChange={(checked) => update('default', checked)}
                />
              </div>
            ) : (
              <ControlLabel label="Default value">
                {(id) => (
                  <input
                    id={id}
                    type={kind === 'number' ? 'number' : 'text'}
                    step={field.field_type === 'number' ? 1 : 'any'}
                    value={String(config.default ?? '')}
                    className={controlClass}
                    onChange={(event) => {
                      if (kind === 'number') {
                        const raw = event.target.value;
                        onError(
                          'default',
                          raw === ''
                            ? 'Enter a numeric default or disable default.'
                            : null,
                        );
                        update('default', raw === '' ? '' : Number(raw));
                      } else {
                        update('default', event.target.value);
                      }
                    }}
                  />
                )}
              </ControlLabel>
            ))}
        </div>
      )}

      {/* Validation Constraints */}
      <div className="space-y-3">
        <h4 className="text-xs font-bold uppercase tracking-wider text-slate-500">
          Validation &amp; Constraints
        </h4>

        <div className="grid gap-3 sm:grid-cols-2">
          {(allowedKeys.has('required') || isCustomType) && (
            <SwitchCard
              label="Required field"
              help="Reject saves unless this field contains a value."
              checked={Boolean(config.required)}
              onChange={(checked) => update('required', checked)}
            />
          )}
          {allowedKeys.has('unique') && (
            <SwitchCard
              label="Unique field"
              help="Disallow duplicate values across collection entries."
              checked={Boolean(config.unique)}
              onChange={(checked) => update('unique', checked)}
            />
          )}
          {allowedKeys.has('preserve_order') && (
            <SwitchCard
              label="Preserve item order"
              help="Keep exact drag/drop order when saving arrays."
              checked={Boolean(config.preserve_order)}
              onChange={(checked) => update('preserve_order', checked)}
            />
          )}
        </div>

        {(allowedKeys.has('min_length') || allowedKeys.has('max_length')) && (
          <div className="grid gap-4 sm:grid-cols-2">
            {allowedKeys.has('min_length') && (
              <ControlLabel label="Minimum length (characters)">
                {(id) => (
                  <input
                    id={id}
                    type="number"
                    min={0}
                    step={1}
                    className={controlClass}
                    value={typeof config.min_length === 'number' ? config.min_length : ''}
                    onChange={(e) =>
                      update(
                        'min_length',
                        e.target.value === '' ? undefined : Number(e.target.value),
                      )
                    }
                  />
                )}
              </ControlLabel>
            )}
            {allowedKeys.has('max_length') && (
              <ControlLabel label="Maximum length (characters)">
                {(id) => (
                  <input
                    id={id}
                    type="number"
                    min={0}
                    step={1}
                    className={controlClass}
                    value={typeof config.max_length === 'number' ? config.max_length : ''}
                    onChange={(e) =>
                      update(
                        'max_length',
                        e.target.value === '' ? undefined : Number(e.target.value),
                      )
                    }
                  />
                )}
              </ControlLabel>
            )}
          </div>
        )}

        {(allowedKeys.has('min') || allowedKeys.has('max')) && (
          <div className="grid gap-4 sm:grid-cols-2">
            {allowedKeys.has('min') && (
              <ControlLabel label="Minimum value">
                {(id) => (
                  <input
                    id={id}
                    type="number"
                    step={field.field_type === 'number' ? 1 : 'any'}
                    className={controlClass}
                    value={typeof config.min === 'number' ? config.min : ''}
                    onChange={(e) =>
                      update('min', e.target.value === '' ? undefined : Number(e.target.value))
                    }
                  />
                )}
              </ControlLabel>
            )}
            {allowedKeys.has('max') && (
              <ControlLabel label="Maximum value">
                {(id) => (
                  <input
                    id={id}
                    type="number"
                    step={field.field_type === 'number' ? 1 : 'any'}
                    className={controlClass}
                    value={typeof config.max === 'number' ? config.max : ''}
                    onChange={(e) =>
                      update('max', e.target.value === '' ? undefined : Number(e.target.value))
                    }
                  />
                )}
              </ControlLabel>
            )}
          </div>
        )}

        {(allowedKeys.has('min_items') || allowedKeys.has('max_items')) && (
          <div className="grid gap-4 sm:grid-cols-2">
            {allowedKeys.has('min_items') && (
              <ControlLabel label="Minimum items">
                {(id) => (
                  <input
                    id={id}
                    type="number"
                    min={0}
                    step={1}
                    className={controlClass}
                    value={typeof config.min_items === 'number' ? config.min_items : ''}
                    onChange={(e) =>
                      update(
                        'min_items',
                        e.target.value === '' ? undefined : Number(e.target.value),
                      )
                    }
                  />
                )}
              </ControlLabel>
            )}
            {allowedKeys.has('max_items') && (
              <ControlLabel label="Maximum items">
                {(id) => (
                  <input
                    id={id}
                    type="number"
                    min={0}
                    step={1}
                    className={controlClass}
                    value={typeof config.max_items === 'number' ? config.max_items : ''}
                    onChange={(e) =>
                      update(
                        'max_items',
                        e.target.value === '' ? undefined : Number(e.target.value),
                      )
                    }
                  />
                )}
              </ControlLabel>
            )}
          </div>
        )}

        {/* Media Allowed Types checkboxes */}
        {allowedKeys.has('allowed_types') && (
          <fieldset className="space-y-2 rounded-xl border border-slate-200 bg-slate-50/50 p-4">
            <legend className="text-xs font-semibold text-slate-700">
              Allowed asset types
            </legend>
            <div className="grid gap-2.5 sm:grid-cols-2">
              {ALLOWED_MEDIA_TYPES.map((mediaOpt) => {
                const selected = Array.isArray(config.allowed_types)
                  ? config.allowed_types
                  : [];
                const checked = selected.includes(mediaOpt.id);
                return (
                  <label
                    key={mediaOpt.id}
                    className={`flex cursor-pointer items-start gap-2.5 rounded-lg border px-3 py-2 text-xs transition-colors ${
                      checked
                        ? 'border-sky-500 bg-sky-50 text-sky-900'
                        : 'border-slate-200 bg-white text-slate-700'
                    }`}
                  >
                    <input
                      type="checkbox"
                      name={`allowed-media-${mediaOpt.id}`}
                      checked={checked}
                      onChange={(e) =>
                        update(
                          'allowed_types',
                          e.target.checked
                            ? [...selected, mediaOpt.id]
                            : selected.filter((t) => t !== mediaOpt.id),
                        )
                      }
                      className="mt-0.5 rounded border-slate-300 text-sky-600 focus:ring-sky-500"
                    />
                    <div>
                      <div className="font-semibold">{mediaOpt.label}</div>
                      <div className="text-[11px] text-slate-500">{mediaOpt.hint}</div>
                    </div>
                  </label>
                );
              })}
            </div>
          </fieldset>
        )}

        {/* URL Allowed Protocols */}
        {allowedKeys.has('allowed_protocols') && (
          <div className="space-y-2 rounded-xl border border-slate-200 bg-slate-50/50 p-4">
            <span className="block text-xs font-semibold text-slate-700">
              Allowed URL protocols (leave empty for http/https default)
            </span>
            <div className="flex flex-wrap gap-2">
              {SAFE_URL_PROTOCOLS.map((proto) => {
                const selected = Array.isArray(config.allowed_protocols)
                  ? config.allowed_protocols
                  : [];
                const active = selected.includes(proto);
                return (
                  <button
                    key={proto}
                    type="button"
                    onClick={() =>
                      update(
                        'allowed_protocols',
                        active
                          ? selected.filter((p) => p !== proto)
                          : [...selected, proto],
                      )
                    }
                    className={`rounded-full border px-3 py-1 text-xs font-semibold transition-colors cursor-pointer ${
                      active
                        ? 'border-sky-500 bg-sky-100 text-sky-800'
                        : 'border-slate-300 bg-white text-slate-600 hover:bg-slate-50'
                    }`}
                  >
                    {proto}://
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {/* Allowed Blocks */}
        {allowedKeys.has('allowed_blocks') && (
          <div className="space-y-2 rounded-xl border border-slate-200 bg-slate-50/50 p-4">
            <span className="block text-xs font-semibold text-slate-700">
              Allowed Tiptap block nodes (empty = all supported blocks)
            </span>
            <div className="flex flex-wrap gap-1.5">
              {BLOCK_TYPE_OPTIONS.map((blk) => {
                const selected = Array.isArray(config.allowed_blocks)
                  ? config.allowed_blocks
                  : [];
                const active = selected.includes(blk.id);
                return (
                  <button
                    key={blk.id}
                    type="button"
                    onClick={() =>
                      update(
                        'allowed_blocks',
                        active
                          ? selected.filter((b) => b !== blk.id)
                          : [...selected, blk.id],
                      )
                    }
                    className={`rounded-lg border px-2.5 py-1 text-xs font-medium transition-colors cursor-pointer ${
                      active
                        ? 'border-sky-500 bg-sky-100 text-sky-800'
                        : 'border-slate-300 bg-white text-slate-600 hover:bg-slate-50'
                    }`}
                  >
                    {blk.label}
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {/* Date Min/Max & Timezone */}
        {(allowedKeys.has('date_min') ||
          allowedKeys.has('date_max') ||
          allowedKeys.has('timezone')) && (
          <div className="grid gap-4 sm:grid-cols-3">
            {allowedKeys.has('date_min') && (
              <ControlLabel
                label="Earliest value"
                help={
                  field.field_type === 'date'
                    ? 'YYYY-MM-DD'
                    : field.field_type === 'time'
                      ? 'HH:MM or HH:MM:SS'
                      : 'ISO-8601 timestamp'
                }
              >
                {(id) => (
                  <input
                    id={id}
                    className={controlClass}
                    value={typeof config.date_min === 'string' ? config.date_min : ''}
                    onChange={(e) => update('date_min', e.target.value || undefined)}
                  />
                )}
              </ControlLabel>
            )}
            {allowedKeys.has('date_max') && (
              <ControlLabel label="Latest value">
                {(id) => (
                  <input
                    id={id}
                    className={controlClass}
                    value={typeof config.date_max === 'string' ? config.date_max : ''}
                    onChange={(e) => update('date_max', e.target.value || undefined)}
                  />
                )}
              </ControlLabel>
            )}
            {allowedKeys.has('timezone') && (
              <ControlLabel label="Timezone" help="IANA timezone (e.g. UTC)">
                {(id) => (
                  <>
                    <input
                      id={id}
                      list={timezoneListId}
                      className={controlClass}
                      value={typeof config.timezone === 'string' ? config.timezone : ''}
                      onChange={(e) => update('timezone', e.target.value || undefined)}
                      placeholder="UTC"
                    />
                    <datalist id={timezoneListId}>
                      {COMMON_TIMEZONES.map((tz) => (
                        <option key={tz} value={tz} />
                      ))}
                    </datalist>
                  </>
                )}
              </ControlLabel>
            )}
          </div>
        )}

        {allowedKeys.has('regex') && (
          <ControlLabel
            label="RegExp pattern"
            help="Server regular expression syntax (Go/RE2)."
          >
            {(id) => (
              <input
                id={id}
                className={`${controlClass} font-mono text-xs`}
                value={typeof config.regex === 'string' ? config.regex : ''}
                onChange={(e) => update('regex', e.target.value || undefined)}
                placeholder="^[a-z0-9-]+$"
              />
            )}
          </ControlLabel>
        )}

        {allowedKeys.has('json_schema') && (
          <JSONSetting
            label="JSON schema"
            value={config.json_schema}
            onChange={(next) => update('json_schema', next)}
            onError={(err) => onError('json_schema', err)}
            objectOnly
          />
        )}

        {(allowedKeys.has('error_message') || isCustomType) && (
          <ControlLabel label="Custom validation error message">
            {(id) => (
              <input
                id={id}
                className={controlClass}
                value={typeof config.error_message === 'string' ? config.error_message : ''}
                onChange={(e) => update('error_message', e.target.value || undefined)}
                placeholder="Optional custom error shown when validation fails"
              />
            )}
          </ControlLabel>
        )}
      </div>

      {/* Visibility & Privacy */}
      <div className="space-y-3 border-t border-slate-100 pt-4">
        <h4 className="text-xs font-bold uppercase tracking-wider text-slate-500">
          Security &amp; Editor Visibility
        </h4>
        <div className="grid gap-3 sm:grid-cols-3">
          {(allowedKeys.has('private') || isCustomType) && (
            <SwitchCard
              label="Private field"
              help="Omit from public API responses."
              checked={Boolean(config.private)}
              onChange={(checked) => update('private', checked)}
            />
          )}
          {(allowedKeys.has('read_only') || isCustomType) && (
            <SwitchCard
              label="Read-only"
              help="Lock input in Content Manager."
              checked={Boolean(config.read_only)}
              onChange={(checked) => update('read_only', checked)}
            />
          )}
          {(allowedKeys.has('hidden') || isCustomType) && (
            <SwitchCard
              label="Hidden"
              help="Hide field from editor form."
              checked={Boolean(config.hidden)}
              onChange={(checked) => update('hidden', checked)}
            />
          )}
        </div>

        {(allowedKeys.has('read_roles') || allowedKeys.has('edit_roles')) && (
          <div className="grid gap-4 sm:grid-cols-2">
            {allowedKeys.has('read_roles') && (
              <StringRows
                label="Restrict read access to roles"
                itemLabel="role"
                values={Array.isArray(config.read_roles) ? config.read_roles : []}
                onChange={(vals) => update('read_roles', vals.length ? vals : undefined)}
              />
            )}
            {allowedKeys.has('edit_roles') && (
              <StringRows
                label="Restrict edit access to roles"
                itemLabel="role"
                values={Array.isArray(config.edit_roles) ? config.edit_roles : []}
                onChange={(vals) => update('edit_roles', vals.length ? vals : undefined)}
              />
            )}
          </div>
        )}
      </div>
    </div>
  );
}
