import React from 'react';
import {
  Type,
  AlignLeft,
  FileText,
  Hash,
  Percent,
  ToggleLeft,
  Calendar,
  Clock,
  CalendarClock,
  Mail,
  Lock,
  Link2,
  ListOrdered,
  Braces,
  Image as ImageIcon,
  GitFork,
  Layers,
  Repeat,
  Blocks,
  Fingerprint,
  LayoutTemplate,
  Sparkles,
} from 'lucide-react';
import type { ContentField, ContentType, FieldType } from '../types';

export interface FieldVisualMeta {
  icon: React.FC<{ className?: string }>;
  badgeBg: string;
  badgeText: string;
  shortLabel: string;
}

export const FIELD_VISUALS: Record<string, FieldVisualMeta> = {
  short_text: { icon: Type, badgeBg: 'bg-emerald-50 border-emerald-200', badgeText: 'text-emerald-700', shortLabel: 'Text' },
  long_text: { icon: AlignLeft, badgeBg: 'bg-emerald-50 border-emerald-200', badgeText: 'text-emerald-700', shortLabel: 'Text' },
  rich_text: { icon: FileText, badgeBg: 'bg-sky-50 border-sky-200', badgeText: 'text-sky-700', shortLabel: 'Rich text (HTML)' },
  blocks: { icon: LayoutTemplate, badgeBg: 'bg-sky-50 border-sky-200', badgeText: 'text-sky-700', shortLabel: 'Rich text (Blocks)' },
  number: { icon: Hash, badgeBg: 'bg-rose-50 border-rose-200', badgeText: 'text-rose-700', shortLabel: 'Number (Integer)' },
  decimal: { icon: Percent, badgeBg: 'bg-rose-50 border-rose-200', badgeText: 'text-rose-700', shortLabel: 'Number (Decimal)' },
  boolean: { icon: ToggleLeft, badgeBg: 'bg-amber-50 border-amber-200', badgeText: 'text-amber-700', shortLabel: 'Boolean' },
  date: { icon: Calendar, badgeBg: 'bg-amber-50 border-amber-200', badgeText: 'text-amber-700', shortLabel: 'Date' },
  time: { icon: Clock, badgeBg: 'bg-amber-50 border-amber-200', badgeText: 'text-amber-700', shortLabel: 'Time' },
  datetime: { icon: CalendarClock, badgeBg: 'bg-amber-50 border-amber-200', badgeText: 'text-amber-700', shortLabel: 'DateTime' },
  email: { icon: Mail, badgeBg: 'bg-rose-50 border-rose-200', badgeText: 'text-rose-700', shortLabel: 'Email' },
  secret: { icon: Lock, badgeBg: 'bg-amber-50 border-amber-200', badgeText: 'text-amber-700', shortLabel: 'Password' },
  url: { icon: Link2, badgeBg: 'bg-teal-50 border-teal-200', badgeText: 'text-teal-700', shortLabel: 'URL' },
  enum: { icon: ListOrdered, badgeBg: 'bg-amber-50 border-amber-200', badgeText: 'text-amber-700', shortLabel: 'Enumeration' },
  json: { icon: Braces, badgeBg: 'bg-indigo-50 border-indigo-200', badgeText: 'text-indigo-700', shortLabel: 'JSON' },
  uid: { icon: Fingerprint, badgeBg: 'bg-indigo-50 border-indigo-200', badgeText: 'text-indigo-700', shortLabel: 'UID' },
  media: { icon: ImageIcon, badgeBg: 'bg-purple-50 border-purple-200', badgeText: 'text-purple-700', shortLabel: 'Media' },
  relation_single: { icon: GitFork, badgeBg: 'bg-sky-50 border-sky-200', badgeText: 'text-sky-700', shortLabel: 'Relation' },
  relation_multiple: { icon: GitFork, badgeBg: 'bg-sky-50 border-sky-200', badgeText: 'text-sky-700', shortLabel: 'Relation' },
  component: { icon: Layers, badgeBg: 'bg-slate-100 border-slate-300', badgeText: 'text-slate-700', shortLabel: 'Component' },
  repeatable_component: { icon: Repeat, badgeBg: 'bg-slate-100 border-slate-300', badgeText: 'text-slate-700', shortLabel: 'Component' },
  dynamic_zone: { icon: Blocks, badgeBg: 'bg-slate-100 border-slate-300', badgeText: 'text-slate-700', shortLabel: 'Dynamic zone' },
};

export function getFieldVisual(fieldType: FieldType | string): FieldVisualMeta {
  return (
    FIELD_VISUALS[fieldType] || {
      icon: Sparkles,
      badgeBg: 'bg-violet-50 border-violet-200',
      badgeText: 'text-violet-700',
      shortLabel: String(fieldType),
    }
  );
}

export const FieldTypeIcon: React.FC<{ fieldType: FieldType | string; size?: 'sm' | 'md' }> = ({
  fieldType,
  size = 'sm',
}) => {
  const meta = getFieldVisual(fieldType);
  const Icon = meta.icon;
  const dim = size === 'sm' ? 'h-7 w-7 rounded-lg' : 'h-9 w-9 rounded-xl';
  const iconDim = size === 'sm' ? 'h-3.5 w-3.5' : 'h-4 w-4';
  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center border ${dim} ${meta.badgeBg} ${meta.badgeText}`}
      aria-hidden="true"
    >
      <Icon className={iconDim} />
    </span>
  );
};

/** Returns a concise Strapi-style summary string of the field's target or subtype. */
export function describeFieldSummary(field: ContentField, contentTypes: ContentType[]): string {
  const cfg = field.configuration || {};
  switch (field.field_type) {
    case 'short_text':
      return 'Short text';
    case 'long_text':
      return 'Long text';
    case 'rich_text':
      return 'Rich text (HTML)';
    case 'blocks':
      return 'Rich text (Blocks)';
    case 'number':
      return 'Integer';
    case 'decimal':
      return 'Decimal';
    case 'boolean':
      return 'Boolean';
    case 'date':
      return 'Date';
    case 'time':
      return 'Time';
    case 'datetime':
      return 'DateTime';
    case 'email':
      return 'Email';
    case 'secret':
      return 'Password';
    case 'url':
      return 'URL';
    case 'uid':
      return cfg.target_field ? `Attached to "${cfg.target_field}"` : 'Manual UID';
    case 'enum': {
      const count = cfg.enum_values?.length || 0;
      return `${count} option${count === 1 ? '' : 's'}`;
    }
    case 'json':
      return 'JSON';
    case 'media': {
      const mode = cfg.multiple ? 'Multiple' : 'Single';
      const allowed = cfg.allowed_types?.length ? ` (${cfg.allowed_types.join(', ')})` : '';
      return `${mode} media${allowed}`;
    }
    case 'relation_single':
    case 'relation_multiple': {
      const target = contentTypes.find((t) => t.uid === cfg.target_type);
      const targetLabel = target ? target.name : cfg.target_type || 'Unset target';
      const rawCard = String(cfg.relation_cardinality || '');
      const card =
        rawCard === 'one_to_one'
          ? 'One-to-One'
          : rawCard === 'one_to_many'
            ? 'One-to-Many'
            : rawCard === 'many_to_one' || field.field_type === 'relation_single'
              ? 'Many-to-One'
              : 'Many-to-Many';
      return `${card} with ${targetLabel}`;
    }
    case 'component':
    case 'repeatable_component': {
      const comp = contentTypes.find((t) => t.uid === cfg.component);
      const label = comp ? comp.name : cfg.component || 'Unconfigured';
      const rep = field.field_type === 'repeatable_component' || cfg.repeatable ? 'Repeatable' : 'Single';
      return `${rep} (${label})`;
    }
    case 'dynamic_zone': {
      const count = cfg.components?.length || 0;
      return `${count} component${count === 1 ? '' : 's'}`;
    }
    default:
      return String(field.field_type);
  }
}
