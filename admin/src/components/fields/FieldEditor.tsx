import { useId } from 'react';
import { Puzzle } from 'lucide-react';
import { useRegisteredCustomFields } from '../../fields/CustomFieldContext';
import { RelationEntryPicker } from '../relations/RelationEntryPicker';
import { BufferedJsonEditor } from './BufferedJsonEditor';
import { ComponentEditor } from './ComponentEditor';
import { MediaEditor } from './MediaEditor';
import { RichTextEditor } from './RichTextEditor';
import { ScalarEditor, UIDEditor } from './ScalarEditor';
import { configFor, fieldPath, inputClass } from './contracts';
import type { FieldEditorProps, SchemaFieldsProps } from './contracts';

export type { FieldEditorProps, SchemaFieldsProps, FieldErrors, ValidityChange } from './contracts';

/** Includes label, help text and errors. Key the containing schema by entry ID. */
export function FieldEditor(props: FieldEditorProps) {
  const id = useId();
  const { field, value, onChange, errors } = props;
  const config = configFor(field);
  const path = props.path || field.field_key;
  const disabled = props.disabled || config.read_only;
  const error = errors?.[path];
  const messages = Array.isArray(error) ? error : error ? [error] : [];
  const help = config.help_text || config.description;
  const describedBy = [help ? `${id}-help` : '', messages.length ? `${id}-errors` : ''].filter(Boolean).join(' ') || undefined;
  const shared = { ...props, id, path, disabled, describedBy, invalid: messages.length > 0 };
  const type: string = field.field_type;
  const customFields = useRegisteredCustomFields();
  const customMeta = customFields.find((cf) => cf.type === type);
  if (config.hidden) return null;
  let editor;
  switch (type) {
    case 'json': editor = <BufferedJsonEditor {...shared} />; break;
    case 'uid': editor = <UIDEditor {...shared} />; break;
    case 'rich_text': case 'blocks': editor = <RichTextEditor {...shared} format={type === 'blocks' ? 'json' : 'html'} />; break;
    case 'media': editor = <MediaEditor {...shared} />; break;
    case 'component': case 'repeatable_component': case 'dynamic_zone': editor = <ComponentEditor {...shared} />; break;
    case 'relation_single': case 'relation_multiple': {
      const multiple = type === 'relation_multiple';
      const valid = value == null || value === '' || (multiple ? Array.isArray(value) && value.every((item) => typeof item === 'string') : typeof value === 'string');
      editor = valid ? <RelationEntryPicker contentTypeUid={config.target_type || ''} multiple={multiple} value={multiple ? (Array.isArray(value) ? value : []) as string[] : typeof value === 'string' && value ? [value] : []} onChange={(ids) => onChange(multiple ? ids : ids[0] || null)} disabled={disabled} /> : <p role="alert" className="text-sm text-amber-700">Legacy relation value preserved. Repair the stored value explicitly before editing relations.</p>;
      break;
    }
    case 'short_text': case 'long_text': case 'number': case 'decimal': case 'boolean': case 'date': case 'time': case 'datetime': case 'email': case 'secret': case 'url': case 'enum': editor = <ScalarEditor {...shared} />; break;
    default: {
      if (customMeta) {
        const isComplex = value !== null && typeof value === 'object';
        editor = (
          <div className="space-y-2 rounded-lg border border-violet-200 bg-violet-50/40 p-3">
            <div className="flex items-center justify-between text-xs text-violet-700">
              <span className="inline-flex items-center gap-1.5 font-semibold">
                <Puzzle className="h-3.5 w-3.5" />
                {customMeta.label || type}
              </span>
              {customMeta.plugin_name && (
                <span className="rounded-full bg-violet-100 px-2 py-0.5 text-[10px] font-semibold">
                  {customMeta.plugin_name}
                </span>
              )}
            </div>
            {isComplex ? (
              <BufferedJsonEditor {...shared} />
            ) : (
              <input
                id={id}
                name={path}
                className={inputClass}
                disabled={disabled}
                required={config.required}
                aria-describedby={describedBy}
                aria-invalid={messages.length > 0}
                placeholder={config.placeholder || `Enter ${customMeta.label || type} value`}
                value={value == null ? '' : String(value)}
                onChange={(e) => onChange(e.target.value)}
              />
            )}
          </div>
        );
      } else {
        editor = <p role="alert" className="text-sm text-amber-700">Unsupported field type: {type}. Stored value is preserved.</p>;
      }
    }
  }
  const hasDirectControl =
    type === 'json' ||
    type === 'uid' ||
    type === 'short_text' ||
    type === 'long_text' ||
    type === 'number' ||
    type === 'decimal' ||
    type === 'boolean' ||
    type === 'date' ||
    type === 'time' ||
    type === 'datetime' ||
    type === 'email' ||
    type === 'secret' ||
    type === 'url' ||
    type === 'enum' ||
    Boolean(customMeta);
  const labelText = (
    <>
      {config.label || field.name || field.field_key}
      {config.required && <span aria-label="required" className="ml-1 text-rose-600">*</span>}
      {config.read_only && <span className="ml-2 text-xs text-slate-500">Read only</span>}
    </>
  );
  const labelClass = props.hideLabel ? 'sr-only' : 'block text-sm font-medium text-slate-800';
  return <div className="min-w-0 space-y-2" role="group" aria-labelledby={`${id}-label`} aria-describedby={describedBy}>
    {hasDirectControl ? (
      <label id={`${id}-label`} htmlFor={id} className={labelClass}>{labelText}</label>
    ) : (
      <div id={`${id}-label`} className={labelClass}>{labelText}</div>
    )}
    {editor}
    {help && <p id={`${id}-help`} className="text-xs text-slate-500">{help}</p>}
    {messages.length > 0 && <div id={`${id}-errors`} role="alert" className="text-xs text-rose-600">{messages.map((message, index) => <p key={index}>{message}</p>)}</div>}
  </div>;
}

/** Recursive schemas use the same field shell, restrictions and path convention. */
export function SchemaFields({ fields, data, onDataChange, path, ...context }: SchemaFieldsProps) {
  let previousGroup: string | undefined;
  return <div className="grid grid-cols-1 gap-4 md:grid-cols-6">{fields.filter((field) => !configFor(field).hidden).map((field) => {
    const config = configFor(field);
    const groupHeading = config.group && config.group !== previousGroup ? config.group : undefined;
    previousGroup = config.group;
    const width = config.layout_width === 'half' ? 'md:col-span-3' : config.layout_width === 'third' ? 'md:col-span-2' : config.layout_width === 'two-thirds' ? 'md:col-span-4' : 'md:col-span-6';
    return <div key={field.id || field.field_key} className={`min-w-0 ${width}`}>{groupHeading && <h3 className="mb-3 text-sm font-semibold text-slate-700">{groupHeading}</h3>}<FieldEditor {...context} field={field} value={data[field.field_key]} data={data} onDataChange={onDataChange} onChange={(value) => onDataChange({ ...data, [field.field_key]: value })} path={fieldPath(path, field.field_key)} /></div>;
  })}</div>;
}
