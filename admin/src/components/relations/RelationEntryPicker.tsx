import React, { useEffect, useMemo, useState } from 'react';
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
  const preferred = ['title', 'name', 'label'];
  for (const key of preferred) { const value = entry.draft_data[key]; if (typeof value === 'string' && value.trim()) return value; }
  const first = Object.values(entry.draft_data).find((value): value is string => typeof value === 'string' && value.trim().length > 0);
  return first || entry.slug || entry.id;
};

export const RelationEntryPicker: React.FC<RelationEntryPickerProps> = ({ contentTypeUid, multiple, value, onChange, disabled }) => {
  const [query, setQuery] = useState(''); const [entries, setEntries] = useState<ContentEntry[]>([]); const [loading, setLoading] = useState(false); const [error, setError] = useState<string | null>(null); const [offset, setOffset] = useState(0); const [total, setTotal] = useState(0);
  const selected = useMemo(() => new Set(value), [value]);
  useEffect(() => { const timer = window.setTimeout(() => { setLoading(true); setError(null); entriesApi.list(contentTypeUid, { limit: 20, offset, preview: true, search: query }).then((response) => { setEntries(response.data || []); setTotal(response.meta?.total || 0); }).catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'Failed to load related entries')).finally(() => setLoading(false)); }, 250); return () => window.clearTimeout(timer); }, [contentTypeUid, offset, query]);
  const toggle = (id: string) => { if (selected.has(id)) onChange(value.filter((entryId) => entryId !== id)); else onChange(multiple ? [...value, id] : [id]); };
  return <div className="space-y-2"><input disabled={disabled || !contentTypeUid} value={query} onChange={(event) => { setQuery(event.target.value); setOffset(0); }} placeholder={`Search ${contentTypeUid || 'entries'}…`} className="w-full px-3 py-2 border border-slate-300 rounded-lg text-sm" />{value.length > 0 && <div className="flex flex-wrap gap-2">{value.map((id) => <span key={id} className="px-2 py-1 rounded bg-indigo-50 border border-indigo-200 text-xs">{entries.find((entry) => entry.id === id) ? labelFor(entries.find((entry) => entry.id === id) as ContentEntry) : id}<button type="button" disabled={disabled} className="ml-2 text-rose-600" onClick={() => toggle(id)} aria-label={`Remove ${id}`}>×</button></span>)}</div>}{loading && <p className="text-xs text-slate-500">Loading entries…</p>}{error && <p role="alert" className="text-xs text-rose-600">{error}</p>}{!loading && !error && entries.length === 0 && <p className="text-xs text-slate-500">No matching entries.</p>}<div className="max-h-40 overflow-y-auto space-y-1">{entries.map((entry) => <button type="button" key={entry.id} disabled={disabled || (!multiple && selected.size > 0 && !selected.has(entry.id)) || selected.has(entry.id)} onClick={() => toggle(entry.id)} className="block w-full px-3 py-2 text-left text-xs rounded bg-slate-50 hover:bg-sky-50 disabled:opacity-50">{labelFor(entry)}</button>)}</div>{total > 20 && <div className="flex items-center justify-between text-xs"><button type="button" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 20))} className="underline">Previous</button><span>{offset + 1}–{Math.min(offset + 20, total)} of {total}</span><button type="button" disabled={offset + 20 >= total} onClick={() => setOffset(offset + 20)} className="underline">Next</button></div>}</div>;
};
