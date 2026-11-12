import type { ContentField, ContentType, ValidationRules } from '../../types';

/** Additive metadata, kept local until the parent updates the shared API types. */
export type FieldConfiguration = ValidationRules & {
  label?: string;
  description?: string;
  help_text?: string;
  placeholder?: string;
  hidden?: boolean;
  read_only?: boolean;
  group?: string;
  default?: unknown;
};

export type FieldErrors = Record<string, string | string[] | undefined>;
export type ValidityChange = (path: string, message: string | null) => void;

export interface FieldEditorProps {
  field: ContentField;
  value: unknown;
  onChange: (value: unknown) => void;
  data: Record<string, unknown>;
  onDataChange?: (data: Record<string, unknown>) => void;
  definitions: ContentType[];
  disabled?: boolean;
  /** Parent manager supplies its own top-level label; nested fields keep theirs. */
  hideLabel?: boolean;
  path?: string;
  depth?: number;
  errors?: FieldErrors;
  onValidityChange?: ValidityChange;
}

export interface SchemaFieldsProps {
  fields: ContentField[];
  data: Record<string, unknown>;
  onDataChange: (data: Record<string, unknown>) => void;
  definitions: ContentType[];
  disabled?: boolean;
  path?: string;
  depth?: number;
  errors?: FieldErrors;
  onValidityChange?: ValidityChange;
}

export const inputClass = 'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 disabled:bg-slate-100 disabled:text-slate-500';
export const buttonClass = 'rounded border border-slate-300 px-2 py-1 text-xs hover:bg-slate-100 disabled:opacity-50';
export const isRecord = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value);
export const configFor = (field: ContentField): FieldConfiguration => field.configuration || {};
export const fieldPath = (parent: string | undefined, key: string) => parent ? `${parent}.${key}` : key;
export const slugify = (value: unknown) => String(value ?? '').toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
export const moveItem = <T,>(items: T[], from: number, to: number): T[] => {
  if (from < 0 || to < 0 || from >= items.length || to >= items.length) return items;
  const result = [...items];
  result.splice(to, 0, result.splice(from, 1)[0]);
  return result;
};

/** Display local wall time while persisting an unambiguous RFC3339 instant. */
export function toLocalDateTime(value: unknown): string {
  if (typeof value !== 'string' || !value) return '';
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return '';
  const pad = (n: number, length = 2) => String(n).padStart(length, '0');
  return `${pad(date.getFullYear(), 4)}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}.${pad(date.getMilliseconds(), 3)}`;
}

export function safeAssetURL(value: string | undefined): string | undefined {
  if (!value || /[\u0000-\u0020]/.test(value)) return undefined;
  if (/^https?:\/\//i.test(value) || (/^\//.test(value) && !/^\/[/\\]/.test(value))) return value;
  return undefined;
}
