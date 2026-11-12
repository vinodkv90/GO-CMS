import React, { useEffect, useId, useMemo, useRef, useState } from 'react';
import { contentTypesApi } from '../../api/endpoints';
import type { ContentType } from '../../types';

interface ContentTypeSelectorProps {
  value: string;
  onChange: (uid: string) => void;
  excludeUid?: string;
  disabled?: boolean;
}

export const ContentTypeSelector: React.FC<ContentTypeSelectorProps> = ({ value, onChange, excludeUid, disabled }) => {
  const [types, setTypes] = useState<ContentType[]>([]);
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [active, setActive] = useState(0);
  const inputId = useId();
  const listId = useId();
  const inputRef = useRef<HTMLInputElement>(null);

  const load = () => {
    setLoading(true); setError(null);
    contentTypesApi.list().then((response) => setTypes(response.data || [])).catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'Failed to load content types')).finally(() => setLoading(false));
  };
  useEffect(load, []);

  const options = useMemo(() => types.filter((type) => type.uid !== excludeUid).filter((type) => `${type.name} ${type.uid}`.toLowerCase().includes(query.toLowerCase())), [types, excludeUid, query]);
  const selected = types.find((type) => type.uid === value);
  const choose = (type: ContentType) => { onChange(type.uid); setQuery(''); setOpen(false); };

  return <div className="relative space-y-1"><label htmlFor={inputId} className="block text-xs font-semibold text-slate-700 tracking-wide uppercase">Target Content Type</label><input id={inputId} ref={inputRef} disabled={disabled} value={open ? query : selected ? `${selected.name} (${selected.uid})` : query} placeholder="Select content type" aria-expanded={open} aria-controls={listId} role="combobox" onFocus={() => { setOpen(true); setQuery(''); }} onChange={(event) => { setQuery(event.target.value); setOpen(true); setActive(0); }} onKeyDown={(event) => { if (!open) return; if (event.key === 'ArrowDown') { event.preventDefault(); setActive((index) => Math.min(index + 1, Math.max(options.length - 1, 0))); } else if (event.key === 'ArrowUp') { event.preventDefault(); setActive((index) => Math.max(index - 1, 0)); } else if (event.key === 'Enter' && options[active]) { event.preventDefault(); choose(options[active]); } else if (event.key === 'Escape') { setOpen(false); inputRef.current?.blur(); } }} className="w-full px-3 py-2 bg-white border border-slate-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-sky-500" />{open && <div id={listId} role="listbox" className="absolute z-20 mt-1 w-full rounded-lg border border-slate-200 bg-white shadow-lg max-h-56 overflow-y-auto">{loading && <p className="p-3 text-xs text-slate-500">Loading content types…</p>}{error && <div className="p-3 text-xs text-rose-700"><p>{error}</p><button type="button" className="mt-1 underline" onClick={load}>Retry</button></div>}{!loading && !error && options.length === 0 && <p className="p-3 text-xs text-slate-500">No content types found.</p>}{!loading && !error && options.map((type, index) => <button type="button" role="option" aria-selected={type.uid === value} key={type.uid} onMouseDown={(event) => event.preventDefault()} onClick={() => choose(type)} className={`block w-full px-3 py-2 text-left ${index === active ? 'bg-sky-50' : 'hover:bg-slate-50'}`}><span className="block text-sm font-semibold text-slate-800">{type.name}</span><span className="block text-[11px] font-mono text-slate-500">{type.uid}</span></button>)}</div>}</div>;
};
