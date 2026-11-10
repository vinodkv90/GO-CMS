import React, { useState } from 'react';
import {
  Layers,
  Plus,
  PlusCircle,
  Hash,
  FileText,
  AlignLeft,
  Calendar,
  CheckSquare,
  Link2,
  Lock,
  Mail,
  Sliders,
  Sparkles,
  Tag,
  Code,
  Image as ImageIcon,
  FolderTree,
  AlertCircle,
} from 'lucide-react';
import { Card, CardHeader, CardContent } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { Modal } from '../components/ui/Modal';
import { contentTypesApi } from '../api/endpoints';
import type { ContentType, FieldType } from '../types';
import { ContentTypeSelector } from '../components/relations/ContentTypeSelector';

export interface ContentTypesViewProps {
  contentTypes: ContentType[];
  onRefresh: () => void;
  onOpenContent: (uid: string) => void;
}

const FIELD_TYPE_OPTIONS: Array<{
  type: FieldType;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  description: string;
}> = [
  { type: 'short_text', label: 'Short Text', icon: FileText, description: 'Single-line text for titles and labels' },
  { type: 'long_text', label: 'Long Text', icon: AlignLeft, description: 'Multi-line plain text or summaries' },
  { type: 'rich_text', label: 'Rich Text', icon: Sparkles, description: 'HTML formatted body content' },
  { type: 'number', label: 'Number', icon: Hash, description: 'Integer values (counts, ordering)' },
  { type: 'decimal', label: 'Decimal', icon: Hash, description: 'Floating-point values (prices, ratings)' },
  { type: 'boolean', label: 'Boolean', icon: CheckSquare, description: 'True/False toggle flag' },
  { type: 'date', label: 'Date', icon: Calendar, description: 'Date only (YYYY-MM-DD)' },
  { type: 'datetime', label: 'Date & Time', icon: Calendar, description: 'Full ISO timestamp' },
  { type: 'email', label: 'Email', icon: Mail, description: 'Validated email address' },
  { type: 'secret', label: 'Secret / Password', icon: Lock, description: 'Hashed or obscured sensitive value' },
  { type: 'url', label: 'URL', icon: Link2, description: 'Validated web address link' },
  { type: 'enum', label: 'Enumeration', icon: Tag, description: 'Constrained list of choices' },
  { type: 'json', label: 'JSON Object', icon: Code, description: 'Arbitrary structured JSON data' },
  { type: 'media', label: 'Media Asset', icon: ImageIcon, description: 'Linked asset from Media Library' },
  { type: 'relation_single', label: 'Single Relation', icon: Link2, description: 'Link to a single related record' },
  { type: 'relation_multiple', label: 'Multiple Relation', icon: FolderTree, description: 'Link to many related records' },
  { type: 'component', label: 'Component', icon: Sliders, description: 'Embedded reusable schema block' },
  { type: 'uid', label: 'UID / Slug', icon: Hash, description: 'Unique slug or identifier' },
];

export const ContentTypesView: React.FC<ContentTypesViewProps> = ({
  contentTypes,
  onRefresh,
  onOpenContent,
}) => {
  // Modal states
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isAddFieldOpen, setIsAddFieldOpen] = useState(false);
  const [targetType, setTargetType] = useState<ContentType | null>(null);

  // Create Type form
  const [newTypeName, setNewTypeName] = useState('');
  const [newTypeUID, setNewTypeUID] = useState('');
  const [newTypeKind, setNewTypeKind] = useState<'collection' | 'single'>('collection');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Add Field form
  const [fieldName, setFieldName] = useState('');
  const [fieldKey, setFieldKey] = useState('');
  const [fieldType, setFieldType] = useState<FieldType>('short_text');
  const [isRequired, setIsRequired] = useState(false);
  const [isUnique, setIsUnique] = useState(false);
  const [minLength, setMinLength] = useState<number | ''>('');
  const [maxLength, setMaxLength] = useState<number | ''>('');
  const [enumValues, setEnumValues] = useState('');
  const [uidSource, setUidSource] = useState('');
  const [relationTarget, setRelationTargetType] = useState('');
  const [mediaMultiple, setMediaMultiple] = useState(false);
  const [allowedMediaTypes, setAllowedMediaTypes] = useState('image');
  const [componentName, setComponentName] = useState('');
  const [componentList, setComponentList] = useState('');

  const handleNameChange = (val: string) => {
    setNewTypeName(val);
    const slug = val
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9]+/g, '_')
      .replace(/^_+|_+$/g, '');
    setNewTypeUID(slug);
  };

  const handleFieldNameChange = (val: string) => {
    setFieldName(val);
    const key = val
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9]+/g, '_')
      .replace(/^_+|_+$/g, '');
    setFieldKey(key);
  };

  const handleCreateType = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newTypeName.trim() || !newTypeUID.trim()) return;

    setError(null);
    setIsSubmitting(true);
    try {
      await contentTypesApi.create({
        name: newTypeName.trim(),
        uid: newTypeUID.trim(),
        kind: newTypeKind,
        fields: [],
      });
      setIsCreateOpen(false);
      setNewTypeName('');
      setNewTypeUID('');
      onRefresh();
    } catch (err: any) {
      setError(err.message || 'Failed to create content type');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleAddField = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!targetType || !fieldName.trim() || !fieldKey.trim()) return;

    setError(null);
    setIsSubmitting(true);
    try {
      const config: Record<string, any> = {
        required: isRequired,
        unique: isUnique,
      };

      if (minLength !== '') config.min_length = Number(minLength);
      if (maxLength !== '') config.max_length = Number(maxLength);
      if (fieldType === 'enum' && enumValues.trim()) {
        config.enum_values = enumValues
          .split(',')
          .map((v) => v.trim())
          .filter(Boolean);
      }
      if (fieldType === 'uid') config.target_field = uidSource;
      if (fieldType === 'media') {
        config.multiple = mediaMultiple;
        config.allowed_types = allowedMediaTypes.split(',').map((v) => v.trim()).filter(Boolean);
      }
      if (fieldType === 'relation_single' || fieldType === 'relation_multiple') config.target_type = relationTarget.trim();
      if (fieldType === 'component' || fieldType === 'repeatable_component') {
        config.component = componentName.trim();
        config.repeatable = fieldType === 'repeatable_component';
      }
      if (fieldType === 'dynamic_zone') config.components = componentList.split(',').map((v) => v.trim()).filter(Boolean);

      await contentTypesApi.addField(targetType.uid, {
        name: fieldName.trim(),
        field_key: fieldKey.trim(),
        field_type: fieldType,
        configuration: config,
      });

      setIsAddFieldOpen(false);
      setFieldName('');
      setFieldKey('');
      setIsRequired(false);
      setIsUnique(false);
      setMinLength('');
      setMaxLength('');
      setEnumValues('');
      setUidSource(''); setRelationTargetType(''); setMediaMultiple(false); setAllowedMediaTypes('image'); setComponentName(''); setComponentList('');
      onRefresh();
    } catch (err: any) {
      setError(err.message || 'Failed to add field');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Top Action Bar */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 tracking-tight">Content Type Builder</h1>
          <p className="text-xs text-slate-500 mt-1">
            Build dynamic collection and single types with 18 built-in field types and schema validation.
          </p>
        </div>
        <Button
          variant="primary"
          onClick={() => {
            setError(null);
            setIsCreateOpen(true);
          }}
        >
          <PlusCircle className="w-4 h-4" />
          <span>Create Content Type</span>
        </Button>
      </div>

      {/* Content Types Grid */}
      <div className="grid grid-cols-1 gap-6">
        {contentTypes.length === 0 ? (
          <Card>
            <CardContent className="p-12 text-center">
              <Layers className="w-12 h-12 text-slate-300 mx-auto mb-3" />
              <h3 className="text-base font-bold text-slate-800">No content types configured</h3>
              <p className="text-xs text-slate-500 mt-1 max-w-sm mx-auto">
                Create your first content model to begin structuring entries for both headless APIs and server-side templates.
              </p>
              <Button
                variant="primary"
                size="sm"
                className="mt-4"
                onClick={() => setIsCreateOpen(true)}
              >
                Create Content Type
              </Button>
            </CardContent>
          </Card>
        ) : (
          contentTypes.map((ct) => (
            <Card key={ct.uid} className="border-slate-200">
              <CardHeader
                title={
                  <div className="flex items-center gap-3">
                    <span className="text-lg font-bold text-slate-900">{ct.name}</span>
                    <Badge variant={ct.kind}>{ct.kind}</Badge>
                    <span className="text-xs font-mono text-slate-400 bg-slate-100 px-2 py-0.5 rounded">
                      uid: {ct.uid}
                    </span>
                  </div>
                }
                description={
                  ct.kind === 'collection'
                    ? 'Collection type with multiple database entries'
                    : 'Single type with one singleton record (e.g. homepage, global settings)'
                }
                action={
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setTargetType(ct);
                        setError(null);
                        setIsAddFieldOpen(true);
                      }}
                    >
                      <Plus className="w-3.5 h-3.5" />
                      <span>Add Field</span>
                    </Button>
                    <Button
                      variant="primary"
                      size="sm"
                      onClick={() => onOpenContent(ct.uid)}
                    >
                      <span>Manage Entries</span>
                    </Button>
                  </div>
                }
              />
              <CardContent className="p-0">
                <div className="overflow-x-auto">
                  <table className="w-full text-left border-collapse text-xs">
                    <thead>
                      <tr className="bg-slate-50/75 border-b border-slate-100 text-slate-500 font-semibold uppercase tracking-wider text-[10px]">
                        <th className="px-6 py-3">Field Name</th>
                        <th className="px-6 py-3">Field Key</th>
                        <th className="px-6 py-3">Type</th>
                        <th className="px-6 py-3">Validation & Constraints</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-100 text-slate-700">
                      {ct.fields && ct.fields.length > 0 ? (
                        ct.fields.map((field) => (
                          <tr key={field.field_key} className="hover:bg-slate-50/50">
                            <td className="px-6 py-3.5 font-bold text-slate-900">{field.name}</td>
                            <td className="px-6 py-3.5 font-mono text-slate-500">{field.field_key}</td>
                            <td className="px-6 py-3.5">
                              <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-mono bg-sky-50 text-sky-700 border border-sky-100">
                                {field.field_type}
                              </span>
                            </td>
                            <td className="px-6 py-3.5 space-x-1.5">
                              {field.configuration?.required && (
                                <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold bg-rose-50 text-rose-700 border border-rose-100">
                                  Required
                                </span>
                              )}
                              {field.configuration?.unique && (
                                <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold bg-indigo-50 text-indigo-700 border border-indigo-100">
                                  Unique
                                </span>
                              )}
                              {field.configuration?.min_length !== undefined && (
                                <span className="inline-block px-1.5 py-0.5 rounded text-[10px] bg-slate-100 text-slate-600">
                                  min: {field.configuration.min_length}
                                </span>
                              )}
                              {field.configuration?.max_length !== undefined && (
                                <span className="inline-block px-1.5 py-0.5 rounded text-[10px] bg-slate-100 text-slate-600">
                                  max: {field.configuration.max_length}
                                </span>
                              )}
                              {field.configuration?.enum_values && (
                                <span className="inline-block px-1.5 py-0.5 rounded text-[10px] bg-amber-50 text-amber-700 border border-amber-100">
                                  values: [{field.configuration.enum_values.join(', ')}]
                                </span>
                              )}
                            </td>
                          </tr>
                        ))
                      ) : (
                        <tr>
                          <td colSpan={4} className="px-6 py-6 text-center text-slate-400 italic">
                            No custom fields defined yet for this content type.
                          </td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              </CardContent>
            </Card>
          ))
        )}
      </div>

      {/* Modal: Create Content Type */}
      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Create Content Type"
        description="Define a new collection or single content structure"
      >
        <form onSubmit={handleCreateType} className="space-y-4">
          {error && (
            <div className="p-3 rounded-lg bg-rose-50 border border-rose-200 flex items-center gap-2 text-xs text-rose-700">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          <div>
            <Input
              label="Display Name"
              placeholder="e.g. Blog Article"
              value={newTypeName}
              onChange={(e) => handleNameChange(e.target.value)}
              required
            />
          </div>

          <div>
            <Input
              label="Unique Identifier (UID / Slug)"
              placeholder="e.g. articles"
              value={newTypeUID}
              onChange={(e) => setNewTypeUID(e.target.value)}
              helperText="Used in REST endpoints: /api/v1/content/:uid"
              required
            />
          </div>

          <div>
            <label className="block text-xs font-semibold text-slate-700 tracking-wide uppercase mb-1.5">
              Type Category
            </label>
            <div className="grid grid-cols-2 gap-3">
              <button
                type="button"
                onClick={() => setNewTypeKind('collection')}
                className={`p-3.5 rounded-xl border text-left cursor-pointer transition-all ${
                  newTypeKind === 'collection'
                    ? 'border-sky-500 bg-sky-50/50 ring-2 ring-sky-500/20'
                    : 'border-slate-200 hover:border-slate-300'
                }`}
              >
                <div className="font-bold text-xs text-slate-900">Collection Type</div>
                <div className="text-[11px] text-slate-500 mt-1">
                  Multiple items (Articles, Products, Categories)
                </div>
              </button>

              <button
                type="button"
                onClick={() => setNewTypeKind('single')}
                className={`p-3.5 rounded-xl border text-left cursor-pointer transition-all ${
                  newTypeKind === 'single'
                    ? 'border-sky-500 bg-sky-50/50 ring-2 ring-sky-500/20'
                    : 'border-slate-200 hover:border-slate-300'
                }`}
              >
                <div className="font-bold text-xs text-slate-900">Single Type</div>
                <div className="text-[11px] text-slate-500 mt-1">
                  Single item (Homepage, About Us, Global Settings)
                </div>
              </button>
            </div>
          </div>

          <div className="flex justify-end gap-2.5 pt-4 border-t border-slate-100">
            <Button
              type="button"
              variant="outline"
              onClick={() => setIsCreateOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant="primary"
              isLoading={isSubmitting}
            >
              Save Content Type
            </Button>
          </div>
        </form>
      </Modal>

      {/* Modal: Add Field */}
      <Modal
        isOpen={isAddFieldOpen}
        onClose={() => setIsAddFieldOpen(false)}
        title={`Add Field to "${targetType?.name}"`}
        description="Choose a field type and configure its constraints"
        maxWidth="xl"
      >
        <form onSubmit={handleAddField} className="space-y-5">
          {error && (
            <div className="p-3 rounded-lg bg-rose-50 border border-rose-200 flex items-center gap-2 text-xs text-rose-700">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {/* Field Type Selector */}
          <div>
            <label className="block text-xs font-semibold text-slate-700 tracking-wide uppercase mb-2">
              Select Field Type
            </label>
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-2.5 max-h-56 overflow-y-auto p-1 border border-slate-200 rounded-xl bg-slate-50/50">
              {FIELD_TYPE_OPTIONS.map((item) => {
                const Icon = item.icon;
                const isSelected = fieldType === item.type;
                return (
                  <button
                    key={item.type}
                    type="button"
                    onClick={() => setFieldType(item.type)}
                    className={`p-2.5 rounded-lg border text-left flex items-start gap-2.5 transition-all cursor-pointer ${
                      isSelected
                        ? 'border-sky-500 bg-white shadow-xs ring-2 ring-sky-500/20'
                        : 'border-slate-200/80 bg-white hover:border-slate-300'
                    }`}
                  >
                    <Icon className={`w-4 h-4 shrink-0 mt-0.5 ${isSelected ? 'text-sky-600' : 'text-slate-400'}`} />
                    <div className="truncate">
                      <div className="text-xs font-bold text-slate-800 truncate">{item.label}</div>
                      <div className="text-[10px] text-slate-400 truncate">{item.type}</div>
                    </div>
                  </button>
                );
              })}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <Input
              label="Field Display Name"
              placeholder="e.g. Hero Subtitle"
              value={fieldName}
              onChange={(e) => handleFieldNameChange(e.target.value)}
              required
            />
            <Input
              label="Field Key"
              placeholder="e.g. hero_subtitle"
              value={fieldKey}
              onChange={(e) => setFieldKey(e.target.value)}
              required
            />
          </div>

          {/* Validation Toggles */}
          <div className="p-4 rounded-xl bg-slate-50 border border-slate-200/80 space-y-3">
            <h4 className="text-xs font-bold text-slate-800 uppercase tracking-wider">Validation Rules</h4>
            <div className="grid grid-cols-2 gap-3 text-xs">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={isRequired}
                  onChange={(e) => setIsRequired(e.target.checked)}
                  className="rounded border-slate-300 text-sky-600 focus:ring-sky-500"
                />
                <span className="font-semibold text-slate-700">Required Field</span>
              </label>

              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={isUnique}
                  onChange={(e) => setIsUnique(e.target.checked)}
                  className="rounded border-slate-300 text-sky-600 focus:ring-sky-500"
                />
                <span className="font-semibold text-slate-700">Unique Value</span>
              </label>
            </div>

            {(fieldType === 'short_text' || fieldType === 'long_text') && (
              <div className="grid grid-cols-2 gap-3 pt-2">
                <Input
                  label="Min Length"
                  type="number"
                  placeholder="0"
                  value={minLength}
                  onChange={(e) => setMinLength(e.target.value === '' ? '' : Number(e.target.value))}
                />
                <Input
                  label="Max Length"
                  type="number"
                  placeholder="255"
                  value={maxLength}
                  onChange={(e) => setMaxLength(e.target.value === '' ? '' : Number(e.target.value))}
                />
              </div>
            )}

            {fieldType === 'enum' && (
              <div className="pt-2">
                <Input
                  label="Enum Allowed Values (Comma Separated)"
                  placeholder="draft, in_review, published, archived"
                  value={enumValues}
                  onChange={(e) => setEnumValues(e.target.value)}
                  helperText="Enter valid choices separated by commas"
                />
              </div>
            )}

            {fieldType === 'uid' && targetType && (
              <div className="pt-2"><label className="block text-xs font-semibold text-slate-700 mb-1">Attached Source Field</label><select value={uidSource} onChange={(e) => setUidSource(e.target.value)} className="w-full px-3 py-2 border rounded-lg text-sm"><option value="">Select a text field</option>{targetType.fields?.filter((f) => ['short_text', 'long_text', 'rich_text'].includes(f.field_type)).map((f) => <option key={f.field_key} value={f.field_key}>{f.name}</option>)}</select></div>
            )}
            {fieldType === 'media' && <div className="pt-2 space-y-3"><label className="flex gap-2 text-xs font-semibold"><input type="checkbox" checked={mediaMultiple} onChange={(e) => setMediaMultiple(e.target.checked)} /> Allow multiple media</label><Input label="Allowed media categories" value={allowedMediaTypes} onChange={(e) => setAllowedMediaTypes(e.target.value)} helperText="Comma-separated: image, video, audio, document" /></div>}
            {(fieldType === 'relation_single' || fieldType === 'relation_multiple') && <div className="pt-2"><ContentTypeSelector value={relationTarget} onChange={setRelationTargetType} excludeUid={targetType?.uid} /></div>}
            {(fieldType === 'component' || fieldType === 'repeatable_component') && <div className="pt-2"><Input label="Component UID" placeholder="e.g. shared.seo" value={componentName} onChange={(e) => setComponentName(e.target.value)} required /></div>}
            {fieldType === 'dynamic_zone' && <div className="pt-2"><Input label="Allowed component UIDs" placeholder="sections.hero, sections.features" value={componentList} onChange={(e) => setComponentList(e.target.value)} required /></div>}
          </div>

          <div className="flex justify-end gap-2.5 pt-3 border-t border-slate-100">
            <Button
              type="button"
              variant="outline"
              onClick={() => setIsAddFieldOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant="primary"
              isLoading={isSubmitting}
            >
              Add Field to Schema
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
