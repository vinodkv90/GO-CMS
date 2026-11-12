import type { ContentField, ContentType, FieldType, ValidationRules } from '../types';

export type SettingSection = 'basic' | 'advanced' | 'validation';
export type SettingControl = 'text' | 'number' | 'checkbox' | 'list' | 'json' | 'default' | 'width' | 'enum' | 'source' | 'relation' | 'component' | 'components' | 'cardinality';
export interface SettingDefinition {
  key: string;
  label: string;
  control: SettingControl;
  section: SettingSection;
  help?: string;
}

const setting = (key: string, label: string, control: SettingControl, section: SettingSection, help?: string): SettingDefinition => ({ key, label, control, section, help });
export const SETTINGS = {
  description: setting('description', 'Description', 'text', 'basic'),
  placeholder: setting('placeholder', 'Placeholder', 'text', 'basic'),
  default: setting('default', 'Default value', 'default', 'basic'),
  layout_width: setting('layout_width', 'Editor width', 'width', 'basic'),
  group: setting('group', 'Field group', 'text', 'basic'),
  hidden: setting('hidden', 'Hidden in editor', 'checkbox', 'advanced'),
  read_only: setting('read_only', 'Read only', 'checkbox', 'advanced'),
  private: setting('private', 'Private (exclude from public responses)', 'checkbox', 'advanced'),
  read_roles: setting('read_roles', 'Read roles', 'list', 'advanced', 'Role identifiers. Leave empty for the normal access rules.'),
  edit_roles: setting('edit_roles', 'Edit roles', 'list', 'advanced', 'Role identifiers. Leave empty for the normal access rules.'),
  required: setting('required', 'Required', 'checkbox', 'validation'),
  unique: setting('unique', 'Unique value', 'checkbox', 'validation'),
  error_message: setting('error_message', 'Validation error message', 'text', 'validation'),
  min_length: setting('min_length', 'Minimum length', 'number', 'validation'),
  max_length: setting('max_length', 'Maximum length', 'number', 'validation'),
  min: setting('min', 'Minimum value', 'number', 'validation'),
  max: setting('max', 'Maximum value', 'number', 'validation'),
  regex: setting('regex', 'Pattern', 'text', 'validation', 'Server regular expression syntax (Go/RE2).'),
  enum_values: setting('enum_values', 'Options', 'enum', 'basic'),
  target_field: setting('target_field', 'UID source field', 'source', 'basic'),
  target_type: setting('target_type', 'Target content type', 'relation', 'basic'),
  multiple: setting('multiple', 'Allow multiple media', 'checkbox', 'basic'),
  allowed_types: setting('allowed_types', 'Allowed media types', 'list', 'validation', 'Supported categories: image, video, audio, file.'),
  min_items: setting('min_items', 'Minimum items', 'number', 'validation'),
  max_items: setting('max_items', 'Maximum items', 'number', 'validation'),
  component: setting('component', 'Component definition', 'component', 'basic'),
  repeatable: setting('repeatable', 'Repeatable component', 'checkbox', 'basic'),
  relation_cardinality: setting('relation_cardinality', 'Relation cardinality', 'cardinality', 'basic'),
  components: setting('components', 'Allowed components', 'components', 'basic'),
  preserve_order: setting('preserve_order', 'Preserve item order', 'checkbox', 'advanced'),
  allowed_protocols: setting('allowed_protocols', 'Allowed URL protocols', 'list', 'validation', 'Protocol names without a colon, for example https, http.'),
  date_min: setting('date_min', 'Earliest value', 'text', 'validation'),
  date_max: setting('date_max', 'Latest value', 'text', 'validation'),
  timezone: setting('timezone', 'Timezone', 'text', 'advanced', 'IANA timezone, for example Asia/Kolkata or UTC.'),
  allowed_blocks: setting('allowed_blocks', 'Allowed block types', 'list', 'validation'),
  json_schema: setting('json_schema', 'JSON schema', 'json', 'validation', 'A JSON schema object; leave empty for no schema restriction.'),
} satisfies Record<string, SettingDefinition>;

export type SettingKey = keyof typeof SETTINGS;
export interface FieldCapability {
  type: FieldType;
  label: string;
  group: 'Text' | 'Numbers & dates' | 'Structured data' | 'References';
  description: string;
  settings: readonly SettingKey[];
  valueKind: 'string' | 'number' | 'boolean' | 'json';
}
const common: SettingKey[] = ['description', 'default', 'layout_width', 'group', 'hidden', 'read_only', 'private', 'read_roles', 'edit_roles', 'required', 'error_message'];
const text: SettingKey[] = ['placeholder', 'unique', 'min_length', 'max_length', 'regex'];
const items: SettingKey[] = ['min_items', 'max_items', 'preserve_order'];
const define = (type: FieldType, label: string, group: FieldCapability['group'], description: string, settings: SettingKey[], valueKind: FieldCapability['valueKind'] = 'string'): FieldCapability => ({ type, label, group, description, settings: [...new Set([...common, ...settings])], valueKind });

/** Local UI capabilities. Server capability comparison can be applied at the API boundary. */
export const FIELD_DEFINITIONS: readonly FieldCapability[] = [
  define('short_text', 'Short text', 'Text', 'Titles and single-line text', text),
  define('long_text', 'Long text', 'Text', 'Multiline plain text', text),
  define('rich_text', 'Rich text', 'Text', 'Formatted HTML content', ['placeholder', 'default', 'min_length', 'max_length', 'regex']),
  define('email', 'Email', 'Text', 'Email address', text),
  define('url', 'URL', 'Text', 'Link with allowed protocols', [...text, 'allowed_protocols']),
  define('secret', 'Secret', 'Text', 'Sensitive text value', ['placeholder', 'min_length', 'max_length', 'regex']),
  define('uid', 'UID / slug', 'Text', 'Identifier generated from a text field', [...text, 'target_field']),
  define('number', 'Number', 'Numbers & dates', 'Integer value', ['placeholder', 'unique', 'min', 'max'], 'number'),
  define('decimal', 'Decimal', 'Numbers & dates', 'Decimal value', ['placeholder', 'unique', 'min', 'max'], 'number'),
  define('boolean', 'Boolean', 'Numbers & dates', 'True or false', ['default'], 'boolean'),
  define('date', 'Date', 'Numbers & dates', 'Calendar date', ['date_min', 'date_max', 'timezone', 'unique']),
  define('datetime', 'Date & time', 'Numbers & dates', 'Timestamp with timezone', ['date_min', 'date_max', 'timezone', 'unique']),
  define('time', 'Time', 'Numbers & dates', 'Time of day', ['date_min', 'date_max', 'timezone', 'unique']),
  define('enum', 'Enumeration', 'Structured data', 'One of a defined set of values', ['enum_values', 'placeholder', 'unique']),
  define('json', 'JSON', 'Structured data', 'Structured JSON value', ['default', 'json_schema'], 'json'),
  define('blocks', 'Blocks', 'Structured data', 'Structured rich content blocks', ['allowed_blocks'], 'json'),
  define('component', 'Component', 'Structured data', 'One reusable group of fields', ['component', 'repeatable', ...items], 'json'),
  define('repeatable_component', 'Repeatable component', 'Structured data', 'An ordered list of reusable groups', ['component', 'repeatable', ...items], 'json'),
  define('dynamic_zone', 'Dynamic zone', 'Structured data', 'Mix allowed component definitions', ['components', ...items], 'json'),
  define('media', 'Media', 'References', 'Assets from the media library', ['multiple', 'allowed_types', ...items], 'json'),
  define('relation_single', 'Single relation', 'References', 'Unidirectional many-to-one link', ['target_type', 'relation_cardinality']),
  define('relation_multiple', 'Multiple relation', 'References', 'Unidirectional many-to-many links', ['target_type', 'relation_cardinality', ...items], 'json'),
];
export const FIELD_REGISTRY = Object.fromEntries(FIELD_DEFINITIONS.map((definition) => [definition.type, definition])) as Record<FieldType, FieldCapability>;
export const FIELD_GROUPS = ['Text', 'Numbers & dates', 'Structured data', 'References'] as const;

export interface ServerFieldCapability { settings: string[] }
export type FieldCapabilityMap = Record<string, ServerFieldCapability>;

export function getFieldSettings(type: FieldType, config: ValidationRules, server?: FieldCapabilityMap): SettingDefinition[] {
  return (FIELD_REGISTRY[type]?.settings || [])
    .filter((key) => !server || server[type]?.settings.includes(key))
    .filter((key) => !(((type === 'media' && !config.multiple) || (type === 'component' && !config.repeatable)) && items.includes(key)))
    .map((key) => SETTINGS[key]);
}

export function initialConfiguration(type: FieldType): ValidationRules {
  const config: ValidationRules = { layout_width: 'full', required: false };
  if (type === 'enum') config.enum_values = [''];
  if (type === 'media') { config.multiple = false; config.allowed_types = ['image']; }
  if (type === 'relation_single') config.relation_cardinality = 'many_to_one';
  if (type === 'relation_multiple') { config.relation_cardinality = 'many_to_many'; config.preserve_order = true; }
  if (type === 'repeatable_component') { config.repeatable = true; config.preserve_order = true; }
  if (type === 'component') config.repeatable = false;
  return config;
}

export function eligibleUIDSources(fields: ContentField[], excludeKey?: string): ContentField[] {
  return fields.filter((field) => field.field_key !== excludeKey && ['short_text', 'long_text', 'rich_text'].includes(field.field_type));
}

export function orderedFields(type: ContentType): ContentField[] {
  const fields = type.fields || [];
  const keys = Array.isArray(type.settings?.field_order) ? type.settings.field_order : [];
  const order = [...new Set(keys.filter((key: unknown): key is string => typeof key === 'string'))];
  return [...order.map((key) => fields.find((field) => field.field_key === key)).filter((field): field is ContentField => Boolean(field)), ...fields.filter((field) => !order.includes(field.field_key))];
}

export function validateFieldDraft(field: ContentField, owner: ContentType, types: ContentType[], editing = false): string[] {
  const errors: string[] = [];
  const config = field.configuration;
  if (!field.name.trim()) errors.push('Enter a field name.');
  if (!editing && !/^[a-z][a-z0-9_]*$/.test(field.field_key)) errors.push('Field keys must start with a letter and contain lowercase letters, numbers or underscores.');
  if (!editing && owner.fields?.some((item) => item.field_key === field.field_key)) errors.push('This field key already exists.');
  for (const [lower, upper] of [['min', 'max'], ['min_length', 'max_length'], ['min_items', 'max_items']] as const) {
    const min = config[lower]; const max = config[upper];
    if (min !== undefined && max !== undefined && min > max) errors.push(`${lower} must not exceed ${upper}.`);
    for (const [key, value] of [[lower, min], [upper, max]] as const) {
      if (value !== undefined && (!Number.isFinite(value) || (key !== 'min' && key !== 'max' && (!Number.isInteger(value) || value < 0)))) errors.push(`${key} must be a non-negative integer.`);
    }
  }
  if (field.field_type === 'enum') {
    const options = config.enum_values || [];
    if (!options.length || options.some((value) => !value.trim())) errors.push('Provide a value for every enumeration option.');
    if (new Set(options).size !== options.length) errors.push('Enumeration options must be unique.');
    if (config.default !== undefined && !options.includes(String(config.default))) errors.push('The default must match an enumeration option.');
  }
  if (field.field_type === 'uid' && config.target_field && !eligibleUIDSources(owner.fields || [], field.field_key).some((item) => item.field_key === config.target_field)) errors.push('Choose an existing text field as the UID source.');
  if (field.field_type.startsWith('relation_')) {
    if (!types.some((type) => type.kind !== 'component' && type.uid === config.target_type)) errors.push('Choose a collection or single type as the relation target.');
    const expected = field.field_type === 'relation_single' ? 'many_to_one' : 'many_to_many';
    if (config.relation_cardinality && config.relation_cardinality !== expected) errors.push(`Only unidirectional ${expected} relations are supported for this field.`);
  }
  const components = types.filter((type) => type.kind === 'component');
  const targets = field.field_type === 'dynamic_zone' ? config.components || [] : ['component', 'repeatable_component'].includes(field.field_type) ? [config.component || ''] : null;
  if (targets && (!targets.length || targets.some((uid) => !components.some((type) => type.uid === uid)))) errors.push('Choose existing component definitions.');
  if (targets?.includes(owner.uid)) errors.push('A component cannot contain itself.');
  if (config.date_min && config.date_max) {
    const min = field.field_type === 'datetime' ? Date.parse(config.date_min) : config.date_min;
    const max = field.field_type === 'datetime' ? Date.parse(config.date_max) : config.date_max;
    if (min > max) errors.push('Earliest value must not exceed latest value.');
  }
  return errors;
}

/** Compare a future GET /field-capabilities map without silently rewriting schemas. */
export function compareFieldCapabilities(server: FieldCapabilityMap) {
  return {
    unavailable: FIELD_DEFINITIONS.filter((field) => !(field.type in server)).map((field) => field.type),
    unknown: Object.keys(server).filter((type) => !(type in FIELD_REGISTRY)),
    settings: FIELD_DEFINITIONS.filter((field) => server[field.type]).flatMap((field) => {
      const remote = server[field.type].settings;
      const unsupported = field.settings.filter((key) => !remote.includes(key));
      const unrepresented = remote.filter((key) => !field.settings.includes(key as SettingKey));
      return unsupported.length || unrepresented.length ? [{ type: field.type, unsupported, unrepresented }] : [];
    }),
  };
}
