import { useEffect, useRef, useState } from 'react';
import { Eye, EyeOff, Lock, RefreshCw, Unlock, X } from 'lucide-react';
import type { FieldEditorProps } from './contracts';
import { buttonClass, configFor, inputClass, slugify, toLocalDateTime } from './contracts';
import { useValidity } from './useValidity';
import { Switch } from '../ui/Switch';

interface Props extends FieldEditorProps {
  id: string;
  describedBy?: string;
  invalid?: boolean;
}

export function UIDEditor({
  field,
  value,
  onChange,
  data,
  disabled,
  id,
  path,
  describedBy,
  invalid,
}: Props) {
  const config = configFor(field);
  const [manual, setManual] = useState(
    () =>
      typeof value === 'string' &&
      value.length > 0 &&
      (!config.target_field || value !== slugify(data[config.target_field])),
  );
  const change = useRef(onChange);
  change.current = onChange;
  const source = config.target_field ? data[config.target_field] : undefined;
  const generated = slugify(source);
  const lastGenerated = useRef<unknown>(value);

  useEffect(() => {
    // A restored or externally supplied value also becomes a manual value.
    if (
      value !== lastGenerated.current &&
      value !== generated &&
      value !== '' &&
      value != null
    ) {
      setManual(true);
      lastGenerated.current = value;
      return;
    }
    if (!manual && !disabled && config.target_field && value !== generated) {
      lastGenerated.current = generated;
      change.current(generated);
    }
  }, [source, generated, value, manual, disabled, config.target_field]);

  const strVal = typeof value === 'string' ? value : '';

  return (
    <div className="space-y-1.5">
      <div className="flex items-stretch gap-2">
        <div className="relative flex-1">
          <input
            id={id}
            name={path || field.field_key}
            className={`${inputClass} pr-24 font-mono ${invalid ? 'border-rose-400 focus:ring-rose-400' : ''}`}
            value={strVal}
            disabled={disabled}
            required={config.required}
            aria-describedby={describedBy}
            aria-invalid={invalid}
            placeholder={config.placeholder || 'my-first-post'}
            onChange={(event) => {
              setManual(true);
              onChange(event.target.value);
            }}
          />
          {config.target_field && (
            <button
              type="button"
              disabled={disabled}
              onClick={() => {
                if (manual) {
                  lastGenerated.current = generated;
                  setManual(false);
                  onChange(generated);
                } else {
                  setManual(true);
                }
              }}
              title={
                manual
                  ? `Locked to manual input. Click to re-link to ${config.target_field}`
                  : `Auto-synced to ${config.target_field}. Click to customize manually`
              }
              className={`absolute top-1/2 right-2 -translate-y-1/2 inline-flex items-center gap-1 rounded px-2 py-0.5 text-[11px] font-semibold transition-colors cursor-pointer disabled:opacity-50 ${
                manual
                  ? 'bg-slate-100 text-slate-600 hover:bg-slate-200'
                  : 'bg-sky-50 text-sky-700 hover:bg-sky-100'
              }`}
            >
              {manual ? (
                <>
                  <Lock className="h-3 w-3" /> Manual
                </>
              ) : (
                <>
                  <Unlock className="h-3 w-3" /> Synced
                </>
              )}
            </button>
          )}
        </div>

        {config.target_field && (
          <button
            type="button"
            className={`${buttonClass} inline-flex items-center gap-1.5 px-3 font-medium text-slate-700 cursor-pointer`}
            disabled={disabled}
            onClick={() => {
              lastGenerated.current = generated;
              setManual(false);
              onChange(generated);
            }}
            aria-label="Regenerate UID"
            title={`Regenerate UID from ${config.target_field}`}
          >
            <RefreshCw className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">Regenerate</span>
          </button>
        )}
      </div>

      {config.target_field && (
        <p className="text-xs text-slate-500">
          {manual
            ? 'Manual value; edits to source field preserve this slug until regenerated.'
            : `Automatically generates from "${config.target_field}".`}
        </p>
      )}
    </div>
  );
}

export function ScalarEditor(props: Props) {
  const {
    field,
    value,
    onChange,
    disabled,
    id,
    describedBy,
    invalid,
    path = field.field_key,
    onValidityChange,
  } = props;
  const config = configFor(field);
  const type: string = field.field_type;
  const [visible, setVisible] = useState(false);
  const [error, setError] = useState<string>();
  const [numberText, setNumberText] = useState(() =>
    value == null ? '' : String(value),
  );
  const lastNumber = useRef(value);

  useEffect(() => {
    if (!Object.is(lastNumber.current, value)) {
      lastNumber.current = value;
      setNumberText(value == null ? '' : String(value));
      setError(undefined);
    }
  }, [value]);

  useValidity(path, disabled ? undefined : error, onValidityChange);

  const errorCls =
    invalid || Boolean(error) ? 'border-rose-400 focus:ring-rose-500' : '';
  const common = {
    id,
    name: path,
    disabled,
    required: config.required,
    'aria-describedby':
      [describedBy, error ? `${id}-input-error` : ''].filter(Boolean).join(' ') ||
      undefined,
    'aria-invalid': invalid || Boolean(error),
    className: `${inputClass} ${errorCls}`.trim(),
  };

  const stringValue = typeof value === 'string' ? value : '';
  const minLen = typeof config.min_length === 'number' ? config.min_length : undefined;
  const maxLen = typeof config.max_length === 'number' ? config.max_length : undefined;
  const hasLengthBounds =
    (type === 'short_text' || type === 'long_text') &&
    (minLen !== undefined || maxLen !== undefined);
  const lengthViolated =
    (minLen !== undefined && stringValue.length > 0 && stringValue.length < minLen) ||
    (maxLen !== undefined && stringValue.length > maxLen);

  let input: React.ReactNode;

  if (type === 'boolean') {
    input = (
      <div className="flex items-center justify-between rounded-lg border border-slate-200 bg-slate-50/70 px-3.5 py-2.5">
        <span className="text-xs text-slate-600">
          {config.placeholder || 'Toggle between False and True'}
        </span>
        <Switch
          id={id}
          checked={value === true}
          onChange={(checked) => onChange(checked)}
          disabled={disabled}
          label={config.label || field.name || field.field_key}
          onLabel="TRUE"
          offLabel="FALSE"
          describedBy={describedBy}
        />
      </div>
    );
  } else if (type === 'enum') {
    const options = config.enum_values || [];
    input = (
      <div className="flex items-center gap-2">
        <select
          {...common}
          value={stringValue}
          onChange={(event) => onChange(event.target.value)}
        >
          <option value="">Choose here...</option>
          {stringValue && !options.includes(stringValue) && (
            <option value={stringValue}>
              {stringValue} (not in current options)
            </option>
          )}
          {options.map((option) => (
            <option key={option} value={option}>
              {option}
            </option>
          ))}
        </select>
        {!disabled && stringValue && !config.required && (
          <button
            type="button"
            onClick={() => onChange('')}
            title="Clear selection"
            aria-label="Clear selection"
            className={`${buttonClass} p-2 text-slate-500 cursor-pointer`}
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </div>
    );
  } else if (type === 'number' || type === 'decimal') {
    input = (
      <input
        {...common}
        className={`${common.className} tabular-nums`}
        type="text"
        inputMode={type === 'number' ? 'numeric' : 'decimal'}
        value={numberText}
        placeholder={config.placeholder || (type === 'decimal' ? '0.00' : '0')}
        onChange={(event) => {
          const text = event.target.value;
          setNumberText(text);
          if (!text.trim()) {
            setError(undefined);
            lastNumber.current = null;
            onChange(null);
            return;
          }
          const number = Number(text);
          const syntax = /^[+-]?(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?$/i.test(text.trim());
          const message =
            !syntax || !Number.isFinite(number)
              ? 'Enter a finite number.'
              : type === 'number' && !Number.isSafeInteger(number)
                ? 'Enter a whole number within the safe integer range.'
                : config.min !== undefined && number < config.min
                  ? `Minimum value is ${config.min}.`
                  : config.max !== undefined && number > config.max
                    ? `Maximum value is ${config.max}.`
                    : undefined;
          setError(message);
          if (!message) {
            lastNumber.current = number;
            onChange(number);
          }
        }}
      />
    );
  } else if (type === 'long_text') {
    input = (
      <textarea
        {...common}
        rows={5}
        value={stringValue}
        minLength={minLen}
        maxLength={maxLen}
        placeholder={config.placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
    );
  } else if (type === 'date' || type === 'time' || type === 'datetime') {
    const displayed = type === 'datetime' ? toLocalDateTime(value) : stringValue;
    const localTz = Intl.DateTimeFormat().resolvedOptions().timeZone;
    input = (
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <input
            {...common}
            type={type === 'datetime' ? 'datetime-local' : type}
            step={type === 'date' ? undefined : '0.001'}
            value={displayed}
            onChange={(event) => {
              const next = event.target.value;
              if (type !== 'datetime' || !next) {
                setError(undefined);
                onChange(next || null);
                return;
              }
              const date = new Date(next);
              if (
                !Number.isFinite(date.getTime()) ||
                !toLocalDateTime(date.toISOString()).startsWith(
                  next.replace(/:00(?:\.000)?$/, ''),
                )
              ) {
                setError('Enter a valid local date and time.');
                return;
              }
              setError(undefined);
              onChange(date.toISOString());
            }}
          />
          {!disabled && displayed && (
            <button
              type="button"
              onClick={() => {
                setError(undefined);
                onChange(null);
              }}
              title="Clear value"
              aria-label="Clear value"
              className={`${buttonClass} p-2 text-slate-500 cursor-pointer`}
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>
        {(type === 'datetime' || config.timezone) && (
          <p className="text-xs text-slate-500">
            Local time ({localTz})
            {config.timezone ? ` • Schema timezone: ${config.timezone}` : ''}
            {type === 'datetime' ? '; stored in UTC.' : '.'}
          </p>
        )}
      </div>
    );
  } else {
    input = (
      <div className="flex gap-2">
        <input
          {...common}
          type={
            type === 'secret'
              ? visible
                ? 'text'
                : 'password'
              : type === 'email'
                ? 'email'
                : type === 'url'
                  ? 'url'
                  : 'text'
          }
          autoComplete={type === 'secret' ? 'new-password' : undefined}
          value={stringValue}
          minLength={minLen}
          maxLength={maxLen}
          pattern={config.regex}
          placeholder={config.placeholder}
          onChange={(event) => onChange(event.target.value)}
        />
        {type === 'secret' && (
          <button
            type="button"
            className={`${buttonClass} inline-flex items-center gap-1.5 px-3 text-slate-700 cursor-pointer`}
            disabled={disabled}
            aria-label={visible ? 'Hide secret' : 'Show secret'}
            aria-pressed={visible}
            onClick={() => setVisible(!visible)}
          >
            {visible ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
            <span>{visible ? 'Hide' : 'Show'}</span>
          </button>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-1">
      {input}
      {hasLengthBounds && (
        <div
          className={`flex justify-end text-[11px] font-medium ${
            lengthViolated ? 'text-rose-600' : 'text-slate-400'
          }`}
        >
          {maxLen !== undefined
            ? `${stringValue.length} / ${maxLen} characters`
            : `${stringValue.length} chars (min ${minLen})`}
        </div>
      )}
      {error && (
        <p id={`${id}-input-error`} role="alert" className="text-xs text-rose-600">
          {error}
        </p>
      )}
    </div>
  );
}
