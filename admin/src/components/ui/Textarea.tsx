import React from 'react';
import { cn } from '../../lib/cn';

export interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  error?: string;
  helperText?: string;
}

export const Textarea: React.FC<TextareaProps> = ({
  label,
  error,
  helperText,
  className = '',
  id,
  rows = 3,
  ...props
}) => {
  const generatedId = React.useId();
  const textareaId = id || generatedId;

  return (
    <div className="space-y-1.5">
      {label && (
        <label htmlFor={textareaId} className="block text-xs font-semibold text-slate-700">
          {label}
          {props.required && <span className="text-rose-500 ml-1">*</span>}
        </label>
      )}
      <textarea
        id={textareaId}
        rows={rows}
        className={cn(
          'w-full px-3 py-2 text-sm bg-white border rounded-lg focus:outline-none focus:ring-2 transition-shadow placeholder:text-slate-400',
          error
            ? 'border-rose-300 focus:ring-rose-500/20 focus:border-rose-500'
            : 'border-slate-300 focus:ring-sky-500/20 focus:border-sky-500',
          'disabled:bg-slate-50 disabled:text-slate-500',
          className,
        )}
        {...props}
      />
      {error && <p className="text-xs text-rose-600">{error}</p>}
      {helperText && !error && <p className="text-xs text-slate-500">{helperText}</p>}
    </div>
  );
};
