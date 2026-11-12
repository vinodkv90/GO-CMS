import { useEffect, useId, useRef } from 'react';
import type { ReactNode } from 'react';

const focusable = 'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, [tabindex="0"]';

export function BuilderDialog({ title, children, onClose, busy = false }: { title: string; children: ReactNode; onClose: () => void; busy?: boolean }) {
  const titleId = useId();
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    ref.current?.focus();
    return () => { document.body.style.overflow = overflow; previous?.focus(); };
  }, []);
  return <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 backdrop-blur-xs p-2 sm:p-5">
    <div ref={ref} role="dialog" aria-modal="true" aria-labelledby={titleId} aria-busy={busy} tabIndex={-1} className="flex max-h-[92vh] w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl outline-none" onKeyDown={(event) => {
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); if (!busy) onClose(); }
      if (event.key === 'Tab') {
        const controls = Array.from(ref.current?.querySelectorAll<HTMLElement>(focusable) || []).filter((element) => !element.closest('fieldset[disabled]') && !element.closest('details:not([open]) :not(summary)'));
        const first = controls[0]; const last = controls.at(-1);
        if (!first) { event.preventDefault(); return; }
        if (event.shiftKey && (document.activeElement === first || document.activeElement === ref.current)) { event.preventDefault(); last?.focus(); }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
      }
    }}>
      <div className="flex items-center justify-between gap-4 border-b border-slate-200 px-5 py-4"><h2 id={titleId} className="text-lg font-semibold text-slate-900">{title}</h2><button type="button" aria-label="Close dialog" disabled={busy} onClick={onClose} className="rounded-lg px-3 py-1 text-xl text-slate-500 hover:bg-slate-100 focus:ring-2 focus:ring-sky-500">×</button></div>
      <div className="overflow-y-auto p-4 sm:p-6">{children}</div>
    </div>
  </div>;
}
