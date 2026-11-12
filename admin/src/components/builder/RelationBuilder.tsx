import React, { useEffect, useId, useMemo, useRef, useState } from 'react';
import { ArrowRight, ChevronDown, Database, FileText, Info, Search } from 'lucide-react';
import type { ContentField, ContentType, FieldType, ValidationRules } from '../../types';

export interface RelationBuilderProps {
  owner: ContentType;
  contentTypes: ContentType[];
  field: ContentField;
  isEditing: boolean;
  onFieldTypeChange: (fieldType: 'relation_single' | 'relation_multiple', nextConfig: ValidationRules) => void;
  onConfigChange: (config: ValidationRules) => void;
  onNameKeyChange?: (name: string, fieldKey?: string) => void;
}

const CARDINALITIES: Array<{
  type: 'relation_single' | 'relation_multiple';
  cardinality: 'many_to_one' | 'many_to_many';
  title: string;
  shortTag: string;
  subtitle: (source: string, target: string) => string;
}> = [
  {
    type: 'relation_single',
    cardinality: 'many_to_one',
    title: 'Many-to-One',
    shortTag: 'Single link',
    subtitle: (source, target) => `${source} belongs to / links to one ${target}`,
  },
  {
    type: 'relation_multiple',
    cardinality: 'many_to_many',
    title: 'Many-to-Many',
    shortTag: 'Multiple links',
    subtitle: (source, target) => `${source} links to multiple ${target} entries`,
  },
];

function CardinalityDiagram({ mode, active }: { mode: 'many_to_one' | 'many_to_many'; active: boolean }) {
  const stroke = active ? 'currentColor' : '#64748b';
  if (mode === 'many_to_one') {
    return (
      <svg viewBox="0 0 64 28" className="h-7 w-14" fill="none" aria-hidden="true">
        <circle cx="10" cy="14" r="4" stroke={stroke} strokeWidth="2" />
        <line x1="14" y1="8" x2="22" y2="14" stroke={stroke} strokeWidth="1.8" strokeLinecap="round" />
        <line x1="14" y1="20" x2="22" y2="14" stroke={stroke} strokeWidth="1.8" strokeLinecap="round" />
        <line x1="14" y1="14" x2="49" y2="14" stroke={stroke} strokeWidth="2" strokeLinecap="round" />
        <rect x="49" y="8" width="10" height="12" rx="2.5" stroke={stroke} strokeWidth="2" fill={active ? 'currentColor' : 'none'} fillOpacity={active ? 0.15 : 0} />
      </svg>
    );
  }
  return (
    <svg viewBox="0 0 64 28" className="h-7 w-14" fill="none" aria-hidden="true">
      <circle cx="9" cy="14" r="3.5" stroke={stroke} strokeWidth="2" />
      <line x1="13" y1="8" x2="21" y2="14" stroke={stroke} strokeWidth="1.8" strokeLinecap="round" />
      <line x1="13" y1="20" x2="21" y2="14" stroke={stroke} strokeWidth="1.8" strokeLinecap="round" />
      <line x1="13" y1="14" x2="51" y2="14" stroke={stroke} strokeWidth="2" strokeLinecap="round" />
      <line x1="51" y1="8" x2="43" y2="14" stroke={stroke} strokeWidth="1.8" strokeLinecap="round" />
      <line x1="51" y1="20" x2="43" y2="14" stroke={stroke} strokeWidth="1.8" strokeLinecap="round" />
      <circle cx="55" cy="14" r="3.5" stroke={stroke} strokeWidth="2" />
    </svg>
  );
}

export const RelationBuilder: React.FC<RelationBuilderProps> = ({
  owner,
  contentTypes,
  field,
  isEditing,
  onFieldTypeChange,
  onConfigChange,
}) => {
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [search, setSearch] = useState('');
  const dropdownRef = useRef<HTMLDivElement>(null);
  const searchId = useId();

  const eligibleTargets = useMemo(
    () =>
      contentTypes
        .filter((t) => t.kind !== 'component')
        .sort((a, b) => a.name.localeCompare(b.name)),
    [contentTypes],
  );

  const filteredTargets = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return eligibleTargets;
    return eligibleTargets.filter(
      (t) => t.name.toLowerCase().includes(q) || t.uid.toLowerCase().includes(q),
    );
  }, [eligibleTargets, search]);

  const targetType = useMemo(
    () => eligibleTargets.find((t) => t.uid === field.configuration.target_type),
    [eligibleTargets, field.configuration.target_type],
  );

  useEffect(() => {
    if (!dropdownOpen) return;
    const handleOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setDropdownOpen(false);
      }
    };
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setDropdownOpen(false);
    };
    document.addEventListener('mousedown', handleOutside);
    document.addEventListener('keydown', handleKey);
    return () => {
      document.removeEventListener('mousedown', handleOutside);
      document.removeEventListener('keydown', handleKey);
    };
  }, [dropdownOpen]);

  const activeMode: FieldType =
    field.field_type === 'relation_multiple' ? 'relation_multiple' : 'relation_single';
  const activeCard =
    CARDINALITIES.find((c) => c.type === activeMode) || CARDINALITIES[0];

  const handleSelectCardinality = (nextType: 'relation_single' | 'relation_multiple', cardinality: 'many_to_one' | 'many_to_many') => {
    if (isEditing && nextType !== field.field_type) return;
    const nextConfig: ValidationRules = {
      ...field.configuration,
      relation_cardinality: cardinality,
    };
    if (nextType === 'relation_single') {
      delete nextConfig.min_items;
      delete nextConfig.max_items;
      delete nextConfig.preserve_order;
    } else if (nextConfig.preserve_order === undefined) {
      nextConfig.preserve_order = true;
    }
    onFieldTypeChange(nextType, nextConfig);
  };

  const targetLabel = targetType?.name || 'Target type';

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 items-stretch gap-3 lg:grid-cols-[1fr_auto_1fr]">
        {/* Left Box: Source Content Type */}
        <div className="flex flex-col justify-between rounded-xl border border-slate-200 bg-slate-50/70 p-4">
          <div className="space-y-2">
            <div className="flex items-center justify-between gap-2">
              <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500">
                Source Model
              </span>
              <span className="inline-flex items-center gap-1 rounded-full bg-sky-100 px-2 py-0.5 text-[11px] font-semibold text-sky-800">
                {owner.kind === 'single' ? <FileText className="h-3 w-3" /> : <Database className="h-3 w-3" />}
                {owner.name}
              </span>
            </div>
            <div className="rounded-lg border border-slate-200 bg-white px-3 py-2.5">
              <div className="text-xs text-slate-500">Field on {owner.name}</div>
              <div className="mt-0.5 font-semibold text-slate-900">
                {field.name.trim() || <span className="text-slate-400 italic">Enter field name above</span>}
              </div>
              {field.field_key && (
                <code className="mt-1 inline-block text-[11px] font-mono text-slate-500">
                  {field.field_key}
                </code>
              )}
            </div>
          </div>
          <p className="mt-3 text-[11px] text-slate-500">
            Stored directly on <code className="font-mono text-slate-700">{owner.uid}</code>
          </p>
        </div>

        {/* Center: Cardinality Diagram Selector */}
        <div className="flex flex-col items-center justify-center gap-2 py-1">
          <div className="flex flex-row gap-2 lg:flex-col">
            {CARDINALITIES.map((card) => {
              const isSelected = activeMode === card.type;
              const disabled = isEditing && !isSelected;
              return (
                <button
                  key={card.type}
                  type="button"
                  disabled={disabled}
                  onClick={() => handleSelectCardinality(card.type, card.cardinality)}
                  title={
                    disabled
                      ? 'Create a new field to switch between single and multiple cardinality'
                      : `${card.title}: ${card.subtitle(owner.name, targetLabel)}`
                  }
                  className={`flex flex-col items-center rounded-xl border px-3 py-2 text-center transition-all ${
                    isSelected
                      ? 'border-sky-500 bg-sky-50 text-sky-700 shadow-xs ring-1 ring-sky-200'
                      : disabled
                        ? 'cursor-not-allowed border-slate-200 bg-slate-100 text-slate-400 opacity-60'
                        : 'cursor-pointer border-slate-200 bg-white text-slate-600 hover:border-sky-300 hover:bg-slate-50'
                  }`}
                >
                  <CardinalityDiagram mode={card.cardinality} active={isSelected} />
                  <span className="mt-1 text-[11px] font-bold">{card.title}</span>
                  <span className="text-[10px] opacity-80">{card.shortTag}</span>
                </button>
              );
            })}
          </div>
        </div>

        {/* Right Box: Target Content Type */}
        <div className="flex flex-col justify-between rounded-xl border border-slate-200 bg-slate-50/70 p-4">
          <div className="space-y-2" ref={dropdownRef}>
            <div className="flex items-center justify-between gap-2">
              <label
                htmlFor={searchId}
                className="text-[11px] font-bold uppercase tracking-wider text-slate-500"
              >
                Target Content Type *
              </label>
              {targetType && (
                <span className="rounded-full bg-indigo-100 px-2 py-0.5 text-[11px] font-semibold text-indigo-800">
                  {targetType.uid}
                </span>
              )}
            </div>

            <div className="relative">
              <button
                id={searchId}
                type="button"
                aria-expanded={dropdownOpen}
                onClick={() => setDropdownOpen((o) => !o)}
                className="flex w-full items-center justify-between rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-left text-sm transition-colors hover:border-slate-400 focus:border-sky-500 focus:outline-none focus:ring-2 focus:ring-sky-500/20 cursor-pointer"
              >
                <span className="truncate font-medium text-slate-900">
                  {targetType ? (
                    <span className="flex items-center gap-2">
                      <span>{targetType.name}</span>
                      <code className="text-xs font-mono text-slate-400">
                        ({targetType.uid})
                      </code>
                    </span>
                  ) : (
                    <span className="text-slate-400">Select a target content type...</span>
                  )}
                </span>
                <ChevronDown className="h-4 w-4 shrink-0 text-slate-400" />
              </button>

              {dropdownOpen && (
                <div className="absolute z-30 mt-1.5 w-full overflow-hidden rounded-xl border border-slate-200 bg-white shadow-xl">
                  <div className="border-b border-slate-100 p-2">
                    <div className="relative">
                      <Search className="pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
                      <input
                        id="ctb-relation-target-search"
                        name="relationTargetSearch"
                        type="search"
                        autoFocus
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                        placeholder="Search collection or single types..."
                        className="w-full rounded-md border border-slate-200 bg-slate-50 py-1.5 pr-2.5 pl-8 text-xs text-slate-900 focus:border-sky-500 focus:bg-white focus:outline-none"
                      />
                    </div>
                  </div>
                  <div className="max-h-52 overflow-y-auto py-1" role="listbox">
                    {filteredTargets.length === 0 ? (
                      <p className="px-3 py-4 text-center text-xs text-slate-500">
                        No matching content types found.
                      </p>
                    ) : (
                      filteredTargets.map((item) => {
                        const selected = item.uid === targetType?.uid;
                        const isSelf = item.uid === owner.uid;
                        return (
                          <button
                            key={item.uid}
                            type="button"
                            role="option"
                            aria-selected={selected}
                            onClick={() => {
                              onConfigChange({
                                ...field.configuration,
                                target_type: item.uid,
                                relation_cardinality:
                                  activeMode === 'relation_multiple'
                                    ? 'many_to_many'
                                    : 'many_to_one',
                              });
                              setDropdownOpen(false);
                              setSearch('');
                            }}
                            className={`flex w-full items-center justify-between px-3 py-2 text-left text-xs transition-colors cursor-pointer ${
                              selected
                                ? 'bg-sky-50 font-semibold text-sky-800'
                                : 'text-slate-700 hover:bg-slate-50'
                            }`}
                          >
                            <div className="min-w-0">
                              <div className="flex items-center gap-1.5 truncate font-medium">
                                <span>{item.name}</span>
                                {isSelf && (
                                  <span className="rounded bg-slate-100 px-1.5 py-0 text-[10px] text-slate-500">
                                    Self
                                  </span>
                                )}
                              </div>
                              <div className="truncate font-mono text-[11px] text-slate-400">
                                {item.uid}
                              </div>
                            </div>
                            <span className="ml-2 shrink-0 rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-medium capitalize text-slate-600">
                              {item.kind}
                            </span>
                          </button>
                        );
                      })
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>

          <p className="mt-3 text-[11px] text-slate-500">
            Entries reference <code className="font-mono text-slate-700">{targetType?.uid || 'target'}</code> IDs
          </p>
        </div>
      </div>

      {/* Live Relationship Preview Bar */}
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-sky-200 bg-sky-50/70 px-3.5 py-2.5 text-xs text-sky-900">
        <div className="flex flex-wrap items-center gap-1.5 font-medium">
          <span className="rounded bg-white px-2 py-0.5 font-semibold text-slate-800 shadow-2xs">
            {owner.name}
          </span>
          <span className="text-sky-600 font-mono">
            ({field.field_key || 'relation'})
          </span>
          <ArrowRight className="h-3.5 w-3.5 text-sky-600" />
          <span className="font-semibold text-sky-800">{activeCard.title}</span>
          <ArrowRight className="h-3.5 w-3.5 text-sky-600" />
          <span className="rounded bg-white px-2 py-0.5 font-semibold text-slate-800 shadow-2xs">
            {targetType ? targetType.name : 'Choose target content type'}
          </span>
        </div>
        <span className="inline-flex items-center gap-1 text-[11px] text-sky-700">
          <Info className="h-3.5 w-3.5 shrink-0" />
          Unidirectional relation (matches Go backend engine)
        </span>
      </div>
    </div>
  );
};

