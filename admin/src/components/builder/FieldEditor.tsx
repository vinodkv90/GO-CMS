import React, { useMemo, useState } from 'react';
import {
  ArrowLeft,
  Blocks,
  Braces,
  Calendar,
  CheckCircle2,
  Fingerprint,
  GitFork,
  Hash,
  Image as ImageIcon,
  Layers,
  Link2,
  ListOrdered,
  Lock,
  Mail,
  Plus,
  Puzzle,
  Search,
  ToggleLeft,
  Type,
  FileText,
  LayoutTemplate,
} from 'lucide-react';
import type { ContentField, ContentType, FieldType } from '../../types';
import {
  FIELD_REGISTRY,
  initialConfiguration,
  validateFieldDraft,
} from '../../fields/registry';
import { FieldTypeIcon } from '../../fields/presentation';
import { useFieldCapabilities } from '../../fields/useFieldCapabilities';
import { Button } from '../ui/Button';
import { Input } from '../ui/Input';
import { FieldSettings } from './FieldSettings';

export interface FieldEditorProps {
  owner: ContentType;
  contentTypes: ContentType[];
  field?: ContentField;
  onSave: (field: ContentField, options?: { addAnother?: boolean }) => Promise<void>;
  onCancel: () => void;
}

interface CatalogCard {
  id: string;
  defaultType: FieldType;
  subtypes?: FieldType[];
  group: 'Text & Rich Content' | 'Numbers, Dates & Selection' | 'Assets & Relationships';
  title: string;
  description: string;
  icon: React.FC<{ className?: string }>;
  badgeBg: string;
  badgeText: string;
}

const DEFAULT_CARDS: CatalogCard[] = [
  {
    id: 'text',
    defaultType: 'short_text',
    subtypes: ['short_text', 'long_text'],
    group: 'Text & Rich Content',
    title: 'Text',
    description: 'Small or long plain text such as titles or descriptions',
    icon: Type,
    badgeBg: 'bg-emerald-50 border-emerald-200',
    badgeText: 'text-emerald-700',
  },
  {
    id: 'rich_text',
    defaultType: 'rich_text',
    group: 'Text & Rich Content',
    title: 'Rich text (HTML)',
    description: 'Formatted Tiptap HTML editor with links & headings',
    icon: FileText,
    badgeBg: 'bg-sky-50 border-sky-200',
    badgeText: 'text-sky-700',
  },
  {
    id: 'blocks',
    defaultType: 'blocks',
    group: 'Text & Rich Content',
    title: 'Rich text (Blocks)',
    description: 'Structured JSON content blocks validated server-side',
    icon: LayoutTemplate,
    badgeBg: 'bg-sky-50 border-sky-200',
    badgeText: 'text-sky-700',
  },
  {
    id: 'email',
    defaultType: 'email',
    group: 'Text & Rich Content',
    title: 'Email',
    description: 'Validated email address input with format check',
    icon: Mail,
    badgeBg: 'bg-rose-50 border-rose-200',
    badgeText: 'text-rose-700',
  },
  {
    id: 'secret',
    defaultType: 'secret',
    group: 'Text & Rich Content',
    title: 'Password',
    description: 'Masked secret field written safely without echoing back',
    icon: Lock,
    badgeBg: 'bg-amber-50 border-amber-200',
    badgeText: 'text-amber-700',
  },
  {
    id: 'url',
    defaultType: 'url',
    group: 'Text & Rich Content',
    title: 'URL',
    description: 'External or internal link with protocol allowlist',
    icon: Link2,
    badgeBg: 'bg-teal-50 border-teal-200',
    badgeText: 'text-teal-700',
  },
  {
    id: 'uid',
    defaultType: 'uid',
    group: 'Text & Rich Content',
    title: 'UID',
    description: 'Unique URL-friendly slug generated from a source field',
    icon: Fingerprint,
    badgeBg: 'bg-indigo-50 border-indigo-200',
    badgeText: 'text-indigo-700',
  },
  {
    id: 'number',
    defaultType: 'number',
    subtypes: ['number', 'decimal'],
    group: 'Numbers, Dates & Selection',
    title: 'Number',
    description: 'Integers or floating-point decimal numbers',
    icon: Hash,
    badgeBg: 'bg-rose-50 border-rose-200',
    badgeText: 'text-rose-700',
  },
  {
    id: 'boolean',
    defaultType: 'boolean',
    group: 'Numbers, Dates & Selection',
    title: 'Boolean',
    description: 'True / False switch toggle (Yes or No)',
    icon: ToggleLeft,
    badgeBg: 'bg-amber-50 border-amber-200',
    badgeText: 'text-amber-700',
  },
  {
    id: 'date',
    defaultType: 'date',
    subtypes: ['date', 'datetime', 'time'],
    group: 'Numbers, Dates & Selection',
    title: 'Date',
    description: 'Calendar date, time of day, or full timezone timestamp',
    icon: Calendar,
    badgeBg: 'bg-amber-50 border-amber-200',
    badgeText: 'text-amber-700',
  },
  {
    id: 'enum',
    defaultType: 'enum',
    group: 'Numbers, Dates & Selection',
    title: 'Enumeration',
    description: 'Dropdown selection from a fixed list of choices',
    icon: ListOrdered,
    badgeBg: 'bg-amber-50 border-amber-200',
    badgeText: 'text-amber-700',
  },
  {
    id: 'json',
    defaultType: 'json',
    group: 'Assets & Relationships',
    title: 'JSON',
    description: 'Structured JSON data with optional JSON Schema',
    icon: Braces,
    badgeBg: 'bg-indigo-50 border-indigo-200',
    badgeText: 'text-indigo-700',
  },
  {
    id: 'media',
    defaultType: 'media',
    group: 'Assets & Relationships',
    title: 'Media',
    description: 'Single or multiple files, images, audio & HLS videos',
    icon: ImageIcon,
    badgeBg: 'bg-purple-50 border-purple-200',
    badgeText: 'text-purple-700',
  },
  {
    id: 'relation',
    defaultType: 'relation_single',
    subtypes: ['relation_single', 'relation_multiple'],
    group: 'Assets & Relationships',
    title: 'Relation',
    description: 'Visual link to another Collection or Single Type',
    icon: GitFork,
    badgeBg: 'bg-sky-50 border-sky-200',
    badgeText: 'text-sky-700',
  },
  {
    id: 'component',
    defaultType: 'component',
    subtypes: ['component', 'repeatable_component'],
    group: 'Assets & Relationships',
    title: 'Component',
    description: 'Reusable group of fields (single or repeatable)',
    icon: Layers,
    badgeBg: 'bg-slate-100 border-slate-300',
    badgeText: 'text-slate-700',
  },
  {
    id: 'dynamic_zone',
    defaultType: 'dynamic_zone',
    group: 'Assets & Relationships',
    title: 'Dynamic zone',
    description: 'Mix & reorder heterogeneous component blocks dynamically',
    icon: Blocks,
    badgeBg: 'bg-slate-100 border-slate-300',
    badgeText: 'text-slate-700',
  },
];

const CARD_GROUPS: CatalogCard['group'][] = [
  'Text & Rich Content',
  'Numbers, Dates & Selection',
  'Assets & Relationships',
];

function autoFieldKey(name: string): string {
  return name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '');
}

export function FieldEditor({
  owner,
  contentTypes,
  field,
  onSave,
  onCancel,
}: FieldEditorProps) {
  const { capabilities, customFields, error: capabilityError } = useFieldCapabilities();
  const isEditing = Boolean(field);

  const [step, setStep] = useState<1 | 2>(field ? 2 : 1);
  const [catalogTab, setCatalogTab] = useState<'default' | 'custom'>('default');
  const [configTab, setConfigTab] = useState<'basic' | 'advanced'>('basic');
  const [search, setSearch] = useState('');

  const [draft, setDraft] = useState<ContentField>(() =>
    field
      ? { ...field, configuration: { ...field.configuration } }
      : {
          name: '',
          field_key: '',
          field_type: 'short_text',
          configuration: initialConfiguration('short_text'),
        },
  );
  const [keyEdited, setKeyEdited] = useState(Boolean(field));
  const [errors, setErrors] = useState<string[]>([]);
  const [inputErrors, setInputErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);

  // Merge active plugin custom fields from `/custom-fields` and any non-built-in keys in `/field-capabilities`
  const mergedCustomFields = useMemo(() => {
    const map = new Map<
      string,
      { type: string; label: string; description: string; pluginName?: string }
    >();
    for (const cf of customFields) {
      map.set(cf.type, {
        type: cf.type,
        label: cf.label || cf.type,
        description:
          cf.description ||
          (cf.plugin_name ? `Provided by ${cf.plugin_name}` : 'Plugin custom field'),
        pluginName: cf.plugin_name || cf.plugin_slug,
      });
    }
    if (capabilities) {
      for (const typeKey of Object.keys(capabilities)) {
        if (!(typeKey in FIELD_REGISTRY) && !map.has(typeKey)) {
          map.set(typeKey, {
            type: typeKey,
            label: typeKey,
            description: 'Plugin-registered custom field type',
          });
        }
      }
    }
    return Array.from(map.values());
  }, [customFields, capabilities]);

  const knownBuiltin = Boolean(FIELD_REGISTRY[draft.field_type]);
  const knownCustom = mergedCustomFields.some((c) => c.type === draft.field_type);
  const knownType = knownBuiltin || knownCustom;

  const changedConfig = Boolean(
    field && JSON.stringify(field.configuration) !== JSON.stringify(draft.configuration),
  );

  const selectCardType = (nextType: FieldType) => {
    setDraft((current) => ({
      ...current,
      field_type: nextType,
      configuration: initialConfiguration(nextType),
    }));
    setErrors([]);
    setInputErrors({});
    setConfigTab('basic');
    setStep(2);
  };

  const close = () => {
    if (saving) return;
    if (
      isEditing &&
      JSON.stringify(draft) !== JSON.stringify(field) &&
      !window.confirm('Discard unsaved field changes?')
    ) {
      return;
    }
    onCancel();
  };

  const handleSubmit = async (addAnother = false) => {
    const next: ContentField = {
      ...draft,
      name: draft.name.trim(),
      field_key: draft.field_key.trim(),
      configuration: { ...draft.configuration },
    };

    const problems = knownBuiltin
      ? [...validateFieldDraft(next, owner, contentTypes, isEditing), ...Object.values(inputErrors)]
      : [
          ...(!next.name ? ['Enter a field name.'] : []),
          ...(!isEditing && !/^[a-z][a-z0-9_]*$/.test(next.field_key)
            ? ['Field keys must start with a letter and contain lowercase letters, numbers or underscores.']
            : []),
          ...(!isEditing && owner.fields?.some((f) => f.field_key === next.field_key)
            ? ['This field key already exists.']
            : []),
          ...Object.values(inputErrors),
        ];

    for (const key of [
      'allowed_types',
      'allowed_protocols',
      'allowed_blocks',
      'read_roles',
      'edit_roles',
    ]) {
      const values = next.configuration[key];
      if (
        Array.isArray(values) &&
        values.some((value) => typeof value !== 'string' || !value.trim())
      ) {
        problems.push(`Remove empty values in ${key}.`);
      }
    }
    if (changedConfig && !acknowledged) {
      problems.push('Acknowledge the schema change notice before saving.');
    }
    if (!knownType) {
      problems.push(
        'This field type is not supported by this builder. Its stored values have been preserved.',
      );
    }

    setErrors(problems);
    if (problems.length > 0) return;

    setSaving(true);
    try {
      await onSave(next, { addAnother });
      if (addAnother) {
        setDraft({
          name: '',
          field_key: '',
          field_type: 'short_text',
          configuration: initialConfiguration('short_text'),
        });
        setKeyEdited(false);
        setErrors([]);
        setInputErrors({});
        setConfigTab('basic');
        setSearch('');
        setStep(1);
      }
    } catch (reason) {
      setErrors([
        reason instanceof Error ? reason.message : 'Failed to save field.',
      ]);
    } finally {
      setSaving(false);
    }
  };

  // Filtered default cards
  const filteredCards = useMemo(() => {
    const q = search.trim().toLowerCase();
    return DEFAULT_CARDS.filter((card) => {
      if (capabilities) {
        const types = card.subtypes || [card.defaultType];
        if (!types.some((t) => Boolean(capabilities[t]))) return false;
      }
      if (!q) return true;
      return `${card.title} ${card.description} ${card.defaultType}`
        .toLowerCase()
        .includes(q);
    });
  }, [search, capabilities]);

  const filteredCustomFields = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return mergedCustomFields;
    return mergedCustomFields.filter((cf) =>
      `${cf.label} ${cf.type} ${cf.description} ${cf.pluginName || ''}`
        .toLowerCase()
        .includes(q),
    );
  }, [mergedCustomFields, search]);

  // --- STEP 1: CATEGORIZED CARD SELECTION ---
  if (step === 1 && !isEditing) {
    return (
      <div className="space-y-5">
        {/* Default vs Custom tabs + Search */}
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-3">
          <div className="inline-flex rounded-lg bg-slate-100 p-1" role="tablist">
            <button
              type="button"
              role="tab"
              aria-selected={catalogTab === 'default'}
              onClick={() => setCatalogTab('default')}
              className={`rounded-md px-3.5 py-1.5 text-xs font-semibold transition-colors cursor-pointer ${
                catalogTab === 'default'
                  ? 'bg-white text-sky-700 shadow-2xs'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              Default ({filteredCards.length})
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={catalogTab === 'custom'}
              onClick={() => setCatalogTab('custom')}
              className={`flex items-center gap-1.5 rounded-md px-3.5 py-1.5 text-xs font-semibold transition-colors cursor-pointer ${
                catalogTab === 'custom'
                  ? 'bg-white text-violet-700 shadow-2xs'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              <Puzzle className="h-3.5 w-3.5" />
              Custom ({mergedCustomFields.length})
            </button>
          </div>

          <div className="relative w-full sm:w-64">
            <Search className="pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
            <input
              id="ctb-field-type-search"
              name="fieldTypeSearch"
              type="search"
              autoFocus
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search field types..."
              aria-label="Search field types"
              className="w-full rounded-lg border border-slate-300 bg-white py-1.5 pr-3 pl-8 text-xs text-slate-900 placeholder:text-slate-400 focus:border-sky-500 focus:outline-none focus:ring-2 focus:ring-sky-500/20"
            />
          </div>
        </div>

        {capabilityError && (
          <p role="status" className="text-xs text-amber-700">
            {capabilityError}
          </p>
        )}

        {catalogTab === 'default' ? (
          filteredCards.length === 0 ? (
            <p className="py-8 text-center text-sm text-slate-500">
              No field types match &ldquo;{search}&rdquo;.
            </p>
          ) : (
            <div className="space-y-5">
              {CARD_GROUPS.map((group) => {
                const cards = filteredCards.filter((c) => c.group === group);
                if (cards.length === 0) return null;
                return (
                  <div key={group} className="space-y-2.5">
                    <h3 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">
                      {group}
                    </h3>
                    <div className="grid gap-2.5 sm:grid-cols-2">
                      {cards.map((card) => {
                        const Icon = card.icon;
                        return (
                          <button
                            key={card.id}
                            type="button"
                            onClick={() => selectCardType(card.defaultType)}
                            className="group flex items-start gap-3 rounded-xl border border-slate-200 bg-white p-3.5 text-left transition-all hover:border-sky-400 hover:bg-sky-50/40 hover:shadow-xs focus:border-sky-500 focus:outline-none focus:ring-2 focus:ring-sky-500/20 cursor-pointer"
                          >
                            <span
                              className={`inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border ${card.badgeBg} ${card.badgeText}`}
                            >
                              <Icon className="h-4.5 w-4.5" />
                            </span>
                            <div className="min-w-0">
                              <div className="text-sm font-semibold text-slate-900 group-hover:text-sky-700">
                                {card.title}
                              </div>
                              <p className="mt-0.5 text-xs leading-relaxed text-slate-500">
                                {card.description}
                              </p>
                            </div>
                          </button>
                        );
                      })}
                    </div>
                  </div>
                );
              })}
            </div>
          )
        ) : filteredCustomFields.length === 0 ? (
          <div className="rounded-xl border border-dashed border-slate-300 bg-slate-50/60 p-8 text-center">
            <div className="mx-auto mb-3 flex h-11 w-11 items-center justify-center rounded-xl bg-violet-100 text-violet-600">
              <Puzzle className="h-5 w-5" />
            </div>
            <h4 className="text-sm font-bold text-slate-900">
              No active plugin custom fields
            </h4>
            <p className="mx-auto mt-1 max-w-md text-xs text-slate-500">
              Custom fields appear automatically when an enabled plugin registers a field type via <code className="font-mono text-slate-700">HostAdapter.RegisterFieldType</code>.
            </p>
          </div>
        ) : (
          <div className="grid gap-2.5 sm:grid-cols-2">
            {filteredCustomFields.map((cf) => (
              <button
                key={cf.type}
                type="button"
                onClick={() => selectCardType(cf.type as FieldType)}
                className="group flex items-start gap-3 rounded-xl border border-slate-200 bg-white p-3.5 text-left transition-all hover:border-violet-400 hover:bg-violet-50/40 cursor-pointer"
              >
                <FieldTypeIcon fieldType={cf.type} size="md" />
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-semibold text-slate-900">
                      {cf.label}
                    </span>
                    {cf.pluginName && (
                      <span className="rounded-full bg-violet-100 px-2 py-0 text-[10px] font-semibold text-violet-700">
                        {cf.pluginName}
                      </span>
                    )}
                  </div>
                  <p className="mt-0.5 text-xs text-slate-500">{cf.description}</p>
                  <code className="mt-1 inline-block text-[11px] font-mono text-slate-400">
                    {cf.type}
                  </code>
                </div>
              </button>
            ))}
          </div>
        )}

        <div className="flex justify-end border-t border-slate-100 pt-4">
          <Button type="button" variant="outline" onClick={close}>
            Cancel
          </Button>
        </div>
      </div>
    );
  }

  // --- STEP 2: FIELD CONFIGURATION WITH BASIC & ADVANCED TABS ---
  const activeLabel =
    FIELD_REGISTRY[draft.field_type]?.label ||
    mergedCustomFields.find((c) => c.type === draft.field_type)?.label ||
    draft.field_type;
  const serverUnsupported = Boolean(
    capabilities && !capabilities[draft.field_type] && !knownCustom,
  );

  return (
    <form
      className="space-y-5"
      onSubmit={(event) => {
        event.preventDefault();
        void handleSubmit(false);
      }}
    >
      {/* Top Field Identity + Tabs Bar */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-3.5">
        <div className="flex items-center gap-3">
          {!isEditing && (
            <button
              type="button"
              onClick={() => {
                setErrors([]);
                setStep(1);
              }}
              className="inline-flex items-center gap-1 rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-100 cursor-pointer"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              Change type
            </button>
          )}
          <FieldTypeIcon fieldType={draft.field_type} size="md" />
          <div>
            <div className="flex items-center gap-2">
              <span className="text-sm font-bold text-slate-900">{activeLabel}</span>
              <code className="rounded bg-slate-100 px-1.5 py-0.5 text-[11px] font-mono text-slate-600">
                {draft.field_type}
              </code>
            </div>
            {isEditing && (
              <p className="text-xs text-slate-500">
                Field type cannot be changed after creation.
              </p>
            )}
          </div>
        </div>

        <div className="inline-flex rounded-lg bg-slate-100 p-1" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={configTab === 'basic'}
            onClick={() => setConfigTab('basic')}
            className={`rounded-md px-3.5 py-1.5 text-xs font-semibold transition-colors cursor-pointer ${
              configTab === 'basic'
                ? 'bg-white text-sky-700 shadow-2xs'
                : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            Basic settings
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={configTab === 'advanced'}
            onClick={() => setConfigTab('advanced')}
            className={`rounded-md px-3.5 py-1.5 text-xs font-semibold transition-colors cursor-pointer ${
              configTab === 'advanced'
                ? 'bg-white text-sky-700 shadow-2xs'
                : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            Advanced settings
          </button>
        </div>
      </div>

      {errors.length > 0 && (
        <div
          role="alert"
          className="rounded-xl border border-rose-200 bg-rose-50 p-3.5 text-xs text-rose-800"
        >
          <ul className="list-disc space-y-1 pl-4">
            {errors.map((error, index) => (
              <li key={index}>{error}</li>
            ))}
          </ul>
        </div>
      )}

      {capabilityError && (
        <p role="status" className="text-xs text-amber-700">
          {capabilityError}
        </p>
      )}

      {serverUnsupported && (
        <p
          role="alert"
          className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800"
        >
          This field type is not advertised as enabled by the current server. Existing values remain safe; saving may be rejected unless enabled.
        </p>
      )}

      <fieldset disabled={saving} className="min-w-0 space-y-5">
        {configTab === 'basic' && (
          <div className="grid gap-4 sm:grid-cols-2">
            <Input
              label="Name"
              required
              autoFocus={!isEditing}
              value={draft.name}
              placeholder="e.g. Title, Hero Banner, Author"
              onChange={(event) => {
                const name = event.target.value;
                setDraft((current) => ({
                  ...current,
                  name,
                  ...(!keyEdited && !isEditing
                    ? { field_key: autoFieldKey(name) }
                    : {}),
                }));
              }}
            />
            <Input
              label="Field key (API attribute)"
              required
              disabled={isEditing}
              value={draft.field_key}
              onChange={(event) => {
                setKeyEdited(true);
                setDraft((current) => ({
                  ...current,
                  field_key: event.target.value,
                }));
              }}
              helperText={
                isEditing
                  ? 'Immutable key protecting existing entry payloads.'
                  : 'Lowercase letters, numbers, and underscores only.'
              }
            />
          </div>
        )}

        {knownType ? (
          <FieldSettings
            key={`${draft.field_type}-${configTab}`}
            field={draft}
            owner={owner}
            contentTypes={contentTypes}
            isEditing={isEditing}
            activeTab={configTab}
            serverCapabilities={capabilities}
            onFieldTypeChange={(nextType, nextConfig) =>
              setDraft((current) => ({
                ...current,
                field_type: nextType,
                configuration: nextConfig,
              }))
            }
            onChange={(configuration) => {
              setDraft((current) => ({ ...current, configuration }));
              setAcknowledged(false);
            }}
            onError={(key, error) =>
              setInputErrors((current) => {
                const next = { ...current };
                if (error) next[key] = error;
                else delete next[key];
                return next;
              })
            }
          />
        ) : (
          <p
            role="alert"
            className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800"
          >
            Unsupported field type &ldquo;{draft.field_type}&rdquo;. Its definition and stored values are preserved.
          </p>
        )}

        {isEditing && (
          <div className="space-y-2 rounded-xl border border-amber-200 bg-amber-50/80 p-3.5 text-xs text-amber-900">
            <p>
              Changing constraints or references does not rewrite or erase existing entry data. Unshown custom configuration keys are preserved.
            </p>
            {changedConfig && (
              <label className="flex cursor-pointer items-center gap-2 font-medium">
                <input
                  id="ctb-field-edit-ack"
                  name="editConfigurationAck"
                  type="checkbox"
                  checked={acknowledged}
                  onChange={(event) => setAcknowledged(event.target.checked)}
                  className="rounded border-amber-400 text-amber-700 focus:ring-amber-500"
                />
                I understand how these configuration changes apply to saved entries.
              </label>
            )}
          </div>
        )}
      </fieldset>

      {/* Footer Actions */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 pt-4">
        <Button type="button" variant="outline" disabled={saving} onClick={close}>
          Cancel
        </Button>

        <div className="flex flex-wrap items-center gap-2">
          {!isEditing && (
            <Button
              type="button"
              variant="secondary"
              disabled={saving || !knownType || serverUnsupported}
              onClick={() => void handleSubmit(true)}
            >
              <Plus className="h-3.5 w-3.5" />
              Add another field
            </Button>
          )}
          <Button
            type="submit"
            isLoading={saving}
            disabled={!knownType || serverUnsupported}
          >
            <CheckCircle2 className="h-4 w-4" />
            {isEditing ? 'Finish & save field' : 'Finish'}
          </Button>
        </div>
      </div>
    </form>
  );
}
