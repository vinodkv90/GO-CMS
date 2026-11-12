import React, { useState, useId } from 'react';
import { Eye, EyeOff, RotateCcw } from 'lucide-react';

export const SECRET_PRESERVE_SENTINEL = '__CMS_SECRET_PRESERVED__';

export interface SecretInputProps {
  id?: string;
  label?: string;
  value: string;
  hasStoredSecret?: boolean;
  onChange: (value: string) => void;
  placeholder?: string;
  helperText?: string;
  error?: string;
  disabled?: boolean;
}

export const SecretInput: React.FC<SecretInputProps> = ({
  id,
  label,
  value,
  hasStoredSecret = false,
  onChange,
  placeholder = 'Enter secret value',
  helperText,
  error,
  disabled = false,
}) => {
  const generatedId = useId();
  const inputId = id || generatedId;
  const [revealed, setRevealed] = useState(false);
  const isPreserved = value === SECRET_PRESERVE_SENTINEL;

  return (
    <div className="space-y-1.5">
      {label && (
        <div className="flex items-center justify-between gap-2">
          <label htmlFor={inputId} className="block text-xs font-semibold text-slate-700">
            {label}
          </label>
          {hasStoredSecret && (
            <span className="text-[10px] font-semibold px-1.5 py-0.5 rounded bg-emerald-50 text-emerald-700 border border-emerald-200">
              Configured
            </span>
          )}
        </div>
      )}

      <div className="relative flex items-center">
        <input
          id={inputId}
          type={revealed && !isPreserved ? 'text' : 'password'}
          autoComplete="new-password"
          data-1p-ignore
          data-lpignore="true"
          value={isPreserved ? '••••••••••••••••' : value}
          disabled={disabled}
          onFocus={() => {
            if (isPreserved) {
              onChange('');
            }
          }}
          onChange={(e) => onChange(e.target.value)}
          placeholder={isPreserved ? 'Configured (leave unchanged or type new secret)' : placeholder}
          className={`w-full pr-20 px-3 py-2 text-sm bg-white border rounded-lg focus:outline-none focus:ring-2 transition-shadow font-mono ${
            error
              ? 'border-rose-300 focus:ring-rose-500/20 focus:border-rose-500'
              : 'border-slate-300 focus:ring-sky-500/20 focus:border-sky-500'
          } disabled:bg-slate-50 disabled:text-slate-500`}
        />

        <div className="absolute right-2 flex items-center gap-1">
          {hasStoredSecret && !isPreserved && (
            <button
              type="button"
              title="Keep existing configured secret"
              onClick={() => onChange(SECRET_PRESERVE_SENTINEL)}
              className="p-1 text-slate-400 hover:text-slate-600 rounded hover:bg-slate-100 cursor-pointer"
            >
              <RotateCcw className="w-3.5 h-3.5" />
            </button>
          )}
          {!isPreserved && (
            <button
              type="button"
              aria-label={revealed ? 'Hide secret' : 'Reveal secret'}
              onClick={() => setRevealed((prev) => !prev)}
              className="p-1 text-slate-400 hover:text-slate-600 rounded hover:bg-slate-100 cursor-pointer"
            >
              {revealed ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
            </button>
          )}
        </div>
      </div>

      {error && <p className="text-xs text-rose-600">{error}</p>}
      {helperText && !error && <p className="text-xs text-slate-500">{helperText}</p>}
    </div>
  );
};
