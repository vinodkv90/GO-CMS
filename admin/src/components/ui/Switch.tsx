import React from 'react';

export interface SwitchProps {
  id?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  label?: string;
  onLabel?: string;
  offLabel?: string;
  describedBy?: string;
  className?: string;
}

export const Switch: React.FC<SwitchProps> = ({
  id,
  checked,
  onChange,
  disabled = false,
  label,
  onLabel = 'ON',
  offLabel = 'OFF',
  describedBy,
  className = '',
}) => {
  return (
    <div className={`inline-flex items-center gap-2.5 ${className}`}>
      <button
        id={id}
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        aria-describedby={describedBy}
        disabled={disabled}
        onClick={() => {
          if (!disabled) onChange(!checked);
        }}
        className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors duration-150 focus:outline-none focus:ring-2 focus:ring-sky-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 ${
          checked ? 'bg-sky-600' : 'bg-slate-300'
        }`}
      >
        <span
          className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-sm ring-0 transition duration-150 ${
            checked ? 'translate-x-5' : 'translate-x-0'
          }`}
        />
      </button>
      <span
        className={`text-xs font-semibold tracking-wide select-none ${
          checked ? 'text-sky-700' : 'text-slate-500'
        }`}
      >
        {checked ? onLabel : offLabel}
      </span>
    </div>
  );
};

