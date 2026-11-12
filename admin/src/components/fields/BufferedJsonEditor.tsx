import { useEffect, useRef, useState } from 'react';
import { buttonClass, inputClass } from './contracts';
import type { ValidityChange } from './contracts';
import { useValidity } from './useValidity';

interface Props {
  id: string;
  value: unknown;
  onChange: (value: unknown) => void;
  disabled?: boolean;
  path: string;
  onValidityChange?: ValidityChange;
  describedBy?: string;
}

const serialize = (value: unknown) => value === undefined ? '' : JSON.stringify(value, null, 2);

export function BufferedJsonEditor({ id, value, onChange, disabled, path, onValidityChange, describedBy }: Props) {
  const [text, setText] = useState(() => serialize(value));
  const [error, setError] = useState<string>();
  const lastValue = useRef(value);
  useEffect(() => {
    if (!Object.is(lastValue.current, value)) {
      lastValue.current = value;
      setText(serialize(value));
      setError(undefined);
    }
  }, [value]);
  useValidity(path, disabled ? undefined : error, onValidityChange);

  const edit = (next: string) => {
    setText(next);
    try {
      const parsed: unknown = next.trim() ? JSON.parse(next) : null;
      lastValue.current = parsed;
      setError(undefined);
      onChange(parsed);
    } catch {
      setError('Enter valid JSON. Your unfinished text has been kept; fix it before saving.');
    }
  };

  return <div className="space-y-2">
    {typeof value === 'string' && <p className="text-xs text-amber-700">This is a stored string, not a structured object. It is preserved as a JSON string; edit explicitly to change its type.</p>}
    <textarea id={id} name={path} className={`${inputClass} font-mono text-xs ${error ? 'border-rose-400 focus:ring-rose-500' : ''}`} rows={8} spellCheck={false} value={text} disabled={disabled} aria-invalid={Boolean(error)} aria-describedby={[describedBy, error ? `${id}-json-error` : ''].filter(Boolean).join(' ') || undefined} onChange={(event) => edit(event.target.value)} />
    <div className="flex items-center justify-between gap-2">
      <button type="button" className={`${buttonClass} cursor-pointer`} disabled={disabled || Boolean(error) || !text.trim()} onClick={() => setText(JSON.stringify(JSON.parse(text), null, 2))}>Format JSON</button>
      {text.trim() && !error && <span className="text-[11px] font-medium text-emerald-600">Valid JSON</span>}
    </div>
    {error && <p id={`${id}-json-error`} role="alert" className="text-xs text-rose-600">{error}</p>}
  </div>;
}
