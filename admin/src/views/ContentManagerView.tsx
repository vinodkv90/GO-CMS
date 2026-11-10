import React, { useState, useEffect, useCallback } from 'react';
import {
  Plus,
  Search,
  History,
  Send,
  EyeOff,
  RotateCcw,
  CheckCircle2,
  Clock,
  AlertCircle,
  ArrowLeft,
  FileEdit,
} from 'lucide-react';
import { Card, CardHeader, CardContent } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { Button } from '../components/ui/Button';
import { Modal } from '../components/ui/Modal';
import { entriesApi } from '../api/endpoints';
import { mediaApi } from '../api/endpoints';
import type { ContentType, ContentEntry, ContentVersion, MediaAsset, ValidationRules } from '../types';
import { RelationEntryPicker } from '../components/relations/RelationEntryPicker';

export interface ContentManagerViewProps {
  contentType: ContentType;
  onBackToDashboard: () => void;
}

const inputClass = 'w-full px-3 py-2 bg-white border border-slate-300 rounded-lg text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-sky-500 focus:border-sky-500';

const MediaPicker: React.FC<{ value: unknown; multiple: boolean; onChange: (value: unknown) => void; allowedTypes?: string[] }> = ({ value, multiple, onChange, allowedTypes }) => {
  const [assets, setAssets] = useState<MediaAsset[]>([]);
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const selected = Array.isArray(value) ? value.map(String) : value ? [String(value)] : [];
  useEffect(() => { if (!open) return; mediaApi.list({ search, limit: 24 }).then((r) => setAssets((r.data || []).filter((a) => !allowedTypes?.length || allowedTypes.some((t) => a.mime_type.startsWith(`${t}/`) || a.mime_type === t)))).catch(() => setAssets([])); }, [open, search, allowedTypes]);
  const choose = (id: string) => { const next = multiple ? Array.from(new Set([...selected, id])) : id; onChange(next); if (!multiple) setOpen(false); };
  return <div className="space-y-2"><div className="flex flex-wrap gap-2">{selected.map((id) => <span key={id} className="px-2 py-1 rounded bg-sky-50 border border-sky-200 text-xs text-sky-800">{id}<button type="button" className="ml-2 text-rose-600" aria-label={`Remove ${id}`} onClick={() => onChange(multiple ? selected.filter((x) => x !== id) : '')}>×</button></span>)}</div><Button type="button" variant="outline" size="sm" onClick={() => setOpen(!open)}>{selected.length ? 'Replace / Add Media' : 'Select from Library'}</Button>{open && <div className="p-3 border rounded-lg bg-slate-50 space-y-2"><input className={inputClass} placeholder="Search media..." value={search} onChange={(e) => setSearch(e.target.value)} />{assets.length === 0 ? <p className="text-xs text-slate-500">No compatible media found.</p> : <div className="grid grid-cols-2 gap-2 max-h-48 overflow-y-auto">{assets.map((asset) => <button type="button" key={asset.id} onClick={() => choose(asset.id)} className="p-2 text-left border rounded bg-white hover:border-sky-400"><span className="block text-xs font-semibold truncate">{asset.original_name}</span><span className="block text-[10px] text-slate-500">{asset.mime_type}</span></button>)}</div>}</div>}</div>;
};


export const ContentManagerView: React.FC<ContentManagerViewProps> = ({
  contentType,
  onBackToDashboard,
}) => {
  const [entries, setEntries] = useState<ContentEntry[]>([]);
  const [totalEntries, setTotalEntries] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');

  // Selected Entry for Editing
  const [activeEntry, setActiveEntry] = useState<ContentEntry | null>(null);
  const [isEditorOpen, setIsEditorOpen] = useState(false);
  const [formData, setFormData] = useState<Record<string, any>>({});
  const [editorError, setEditorError] = useState<string | null>(null);
  const [jsonErrors, setJsonErrors] = useState<Record<string, string>>({});
  const [isSaving, setIsSaving] = useState(false);

  // Version History Modal
  const [isVersionModalOpen, setIsVersionModalOpen] = useState(false);
  const [versionHistory, setVersionHistory] = useState<ContentVersion[]>([]);
  const [isLoadingVersions, setIsLoadingVersions] = useState(false);

  const fetchEntries = useCallback(async () => {
    setIsLoading(true);
    try {
      const res = await entriesApi.list(contentType.uid, { limit: 50, preview: true });
      setEntries(res.data || []);
      setTotalEntries(res.meta?.total || (res.data ? res.data.length : 0));

      // If it's a single type and there's already an entry, prepare it
      if (contentType.kind === 'single' && res.data && res.data.length > 0) {
        const single = res.data[0];
        setActiveEntry(single);
        setFormData(single.draft_data || {});
      }
    } catch (err: any) {
      console.error('Failed to load entries:', err);
    } finally {
      setIsLoading(false);
    }
  }, [contentType.uid, contentType.kind]);

  useEffect(() => {
    fetchEntries();
  }, [fetchEntries]);

  // Open editor for a new entry
  const handleCreateNew = () => {
    const initial: Record<string, any> = {};
    contentType.fields?.forEach((f) => {
      if (f.field_type === 'boolean') initial[f.field_key] = false;
      else if (f.field_type === 'number' || f.field_type === 'decimal') initial[f.field_key] = 0;
      else initial[f.field_key] = '';
    });
    setActiveEntry(null);
    setFormData(initial);
    setEditorError(null);
    setJsonErrors({});
    setIsEditorOpen(true);
  };

  // Open editor for existing entry
  const handleEditEntry = (entry: ContentEntry) => {
    setActiveEntry(entry);
    setFormData(entry.draft_data || {});
    setEditorError(null);
    setJsonErrors({});
    setIsEditorOpen(true);
  };

  // Save draft
  const handleSaveDraft = async () => {
    setEditorError(null);
    if (Object.keys(jsonErrors).length > 0) {
      setEditorError('Fix invalid JSON before saving.');
      return;
    }
    setIsSaving(true);
    try {
      if (activeEntry) {
        // Update with OCC (optimistic concurrency control)
        const updated = await entriesApi.update(
          contentType.uid,
          activeEntry.id,
          formData,
          activeEntry.draft_version
        );
        setActiveEntry(updated.data);
      } else {
        // Create new
        const created = await entriesApi.create(contentType.uid, formData);
        setActiveEntry(created.data);
      }
      await fetchEntries();
      if (contentType.kind === 'collection') {
        setIsEditorOpen(false);
      }
    } catch (err: any) {
      setEditorError(err.message || 'Failed to save entry draft');
    } finally {
      setIsSaving(false);
    }
  };

  // Publish entry
  const handlePublish = async (entry: ContentEntry) => {
    try {
      const res = await entriesApi.publish(contentType.uid, entry.id);
      setActiveEntry(res.data);
      await fetchEntries();
    } catch (err: any) {
      alert(`Publish failed: ${err.message}`);
    }
  };

  // Unpublish entry
  const handleUnpublish = async (entry: ContentEntry) => {
    try {
      const res = await entriesApi.unpublish(contentType.uid, entry.id);
      setActiveEntry(res.data);
      await fetchEntries();
    } catch (err: any) {
      alert(`Unpublish failed: ${err.message}`);
    }
  };

  // Open Version History Modal
  const handleOpenVersions = async (entry: ContentEntry) => {
    setActiveEntry(entry);
    setIsVersionModalOpen(true);
    setIsLoadingVersions(true);
    try {
      const res = await entriesApi.versions(contentType.uid, entry.id);
      setVersionHistory(res.data || []);
    } catch (err: any) {
      console.error('Failed to load version history:', err);
    } finally {
      setIsLoadingVersions(false);
    }
  };

  // Restore snapshot version
  const handleRestoreVersion = async (versionNumber: number) => {
    if (!activeEntry) return;
    try {
      const restored = await entriesApi.restore(contentType.uid, activeEntry.id, versionNumber);
      setActiveEntry(restored.data);
      setFormData(restored.data.draft_data || {});
      setIsVersionModalOpen(false);
      await fetchEntries();
    } catch (err: any) {
      alert(`Restore failed: ${err.message}`);
    }
  };

  // Filter entries
  const filteredEntries = entries.filter((e) => {
    if (!searchQuery.trim()) return true;
    const jsonStr = JSON.stringify(e.draft_data).toLowerCase();
    return jsonStr.includes(searchQuery.toLowerCase()) || e.id.includes(searchQuery);
  });

  // Render input field based on schema field type
  const renderFieldInput = (fieldKey: string, fieldType: string, config?: ValidationRules) => {
    const value = formData[fieldKey] ?? '';

    switch (fieldType) {
      case 'boolean':
        return (
          <label className="flex items-center gap-2 cursor-pointer mt-2">
            <input
              type="checkbox"
              checked={Boolean(value)}
              onChange={(e) => setFormData({ ...formData, [fieldKey]: e.target.checked })}
              className="rounded border-slate-300 text-sky-600 focus:ring-sky-500 w-4 h-4"
            />
            <span className="text-xs font-semibold text-slate-700">Enabled</span>
          </label>
        );

      case 'long_text':
      case 'rich_text':
        return (
          <textarea
            rows={fieldType === 'rich_text' ? 6 : 3}
            value={value}
            onChange={(e) => setFormData({ ...formData, [fieldKey]: e.target.value })}
            className="w-full px-3 py-2 bg-white border border-slate-300 rounded-lg text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-sky-500 focus:border-sky-500 font-sans"
            placeholder={`Enter ${fieldKey}...`}
          />
        );

      case 'enum':
        const options: string[] = config?.enum_values || [];
        return (
          <select
            value={value}
            onChange={(e) => setFormData({ ...formData, [fieldKey]: e.target.value })}
            className="w-full px-3 py-2 bg-white border border-slate-300 rounded-lg text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-sky-500 focus:border-sky-500 cursor-pointer"
          >
            <option value="">-- Select an option --</option>
            {options.map((opt) => (
              <option key={opt} value={opt}>
                {opt}
              </option>
            ))}
          </select>
        );

      case 'number':
      case 'decimal':
        return (
          <input
            type="number"
            step={fieldType === 'decimal' ? '0.01' : '1'}
            value={value}
            onChange={(e) =>
              setFormData({
                ...formData,
                [fieldKey]: e.target.value === '' ? '' : Number(e.target.value),
              })
            }
            className="w-full px-3 py-2 bg-white border border-slate-300 rounded-lg text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-sky-500 focus:border-sky-500"
          />
        );

      case 'date':
        return (
          <input
            type="date"
            value={value}
            onChange={(e) => setFormData({ ...formData, [fieldKey]: e.target.value })}
            className="w-full px-3 py-2 bg-white border border-slate-300 rounded-lg text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-sky-500 focus:border-sky-500"
          />
        );

      case 'datetime':
        return (
          <input
            type="datetime-local"
            value={value}
            onChange={(e) => setFormData({ ...formData, [fieldKey]: e.target.value })}
            className="w-full px-3 py-2 bg-white border border-slate-300 rounded-lg text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-sky-500 focus:border-sky-500"
          />
        );

      case 'json':
        return (
          <textarea
            rows={4}
            value={typeof value === 'object' ? JSON.stringify(value, null, 2) : value}
            onChange={(e) => {
              try {
                const parsed = JSON.parse(e.target.value);
                setFormData({ ...formData, [fieldKey]: parsed });
                setJsonErrors((current) => { const next = { ...current }; delete next[fieldKey]; return next; });
              } catch {
                setJsonErrors((current) => ({ ...current, [fieldKey]: 'Invalid JSON syntax' }));
              }
            }}
            className="w-full px-3 py-2 bg-slate-900 text-slate-100 font-mono text-xs rounded-lg border border-slate-700 focus:outline-none focus:ring-2 focus:ring-sky-500"
            placeholder="{}"
          />
        );

      case 'uid':
        return <div className="flex gap-2"><input className={inputClass} value={String(value)} onChange={(e) => setFormData({ ...formData, [fieldKey]: e.target.value })} placeholder="my-first-post" /><Button type="button" variant="outline" size="sm" onClick={() => { const source = config?.target_field; const raw = source ? String(formData[source] || '') : ''; setFormData({ ...formData, [fieldKey]: raw.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') }); }}>Regenerate</Button></div>;

      case 'media':
        return <MediaPicker value={value} multiple={Boolean(config?.multiple)} allowedTypes={config?.allowed_types} onChange={(next) => setFormData({ ...formData, [fieldKey]: next })} />;

      case 'relation_single':
      case 'relation_multiple':
        return <RelationEntryPicker contentTypeUid={config?.target_type || ''} multiple={fieldType === 'relation_multiple'} value={Array.isArray(value) ? value.map(String) : value ? [String(value)] : []} onChange={(next) => setFormData({ ...formData, [fieldKey]: fieldType === 'relation_multiple' ? next : (next[0] || '') })} />;

      case 'component':
      case 'repeatable_component':
      case 'dynamic_zone':
        return <textarea rows={6} className="w-full px-3 py-2 bg-slate-900 text-slate-100 font-mono text-xs rounded-lg" value={typeof value === 'string' ? value : JSON.stringify(value || (fieldType === 'component' ? {} : []), null, 2)} onChange={(e) => { try { setFormData({ ...formData, [fieldKey]: JSON.parse(e.target.value) }); } catch { /* keep editor text until valid JSON */ } }} placeholder={fieldType === 'dynamic_zone' ? '[]' : '{}'} aria-label={`${fieldKey} structured value`} />;

      case 'short_text':
      case 'email':
      case 'url':
      case 'secret':
        return (
          <input
            type={fieldType === 'secret' ? 'password' : 'text'}
            value={value}
            onChange={(e) => setFormData({ ...formData, [fieldKey]: e.target.value })}
            placeholder={`Enter ${fieldKey}...`}
            className="w-full px-3 py-2 bg-white border border-slate-300 rounded-lg text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-sky-500 focus:border-sky-500"
          />
        );

      default:
        return <div role="alert" className="p-3 rounded-lg border border-amber-200 bg-amber-50 text-xs text-amber-800">Unsupported field type <code>{fieldType}</code>. This value cannot be edited as text.</div>;
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" onClick={onBackToDashboard}>
            <ArrowLeft className="w-4 h-4" />
          </Button>
          <div>
            <div className="flex items-center gap-2.5">
              <h1 className="text-2xl font-bold text-slate-900 tracking-tight">{contentType.name}</h1>
              <Badge variant={contentType.kind}>{contentType.kind}</Badge>
            </div>
            <p className="text-xs text-slate-500 mt-0.5 font-mono">
              /api/v1/content/{contentType.uid} • {contentType.fields?.length || 0} fields
            </p>
          </div>
        </div>

        {contentType.kind === 'collection' && (
          <Button variant="primary" onClick={handleCreateNew}>
            <Plus className="w-4 h-4" />
            <span>New Entry</span>
          </Button>
        )}
      </div>

      {/* SINGLE TYPE WORKFLOW: Direct Editor Screen */}
      {contentType.kind === 'single' ? (
        <Card>
          <CardHeader
            title="Single Content Record"
            description="Edit draft data, control optimistic locking version, and publish to the live site."
            action={
              <div className="flex items-center gap-2">
                {activeEntry && (
                  <>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleOpenVersions(activeEntry)}
                    >
                      <History className="w-3.5 h-3.5" />
                      <span>Version History (v{activeEntry.draft_version})</span>
                    </Button>
                    {activeEntry.status === 'published' ? (
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => handleUnpublish(activeEntry)}
                      >
                        <EyeOff className="w-3.5 h-3.5 text-amber-600" />
                        <span>Unpublish</span>
                      </Button>
                    ) : (
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => handlePublish(activeEntry)}
                      >
                        <Send className="w-3.5 h-3.5" />
                        <span>Publish Live</span>
                      </Button>
                    )}
                  </>
                )}
                <Button
                  variant="primary"
                  size="sm"
                  onClick={handleSaveDraft}
                  isLoading={isSaving}
                >
                  <CheckCircle2 className="w-3.5 h-3.5" />
                  <span>Save Draft</span>
                </Button>
              </div>
            }
          />
          <CardContent className="space-y-6">
            {editorError && (
              <div className="p-3.5 rounded-lg bg-rose-50 border border-rose-200 flex items-center gap-2 text-xs text-rose-700">
                <AlertCircle className="w-4 h-4 shrink-0" />
                <span>{editorError}</span>
              </div>
            )}

            {activeEntry && (
              <div className="p-3.5 rounded-xl bg-slate-50 border border-slate-200 flex items-center justify-between text-xs">
                <div className="flex items-center gap-3">
                  <Badge variant={activeEntry.status}>{activeEntry.status}</Badge>
                  <span className="text-slate-600">
                    Draft v{activeEntry.draft_version} • Live v{activeEntry.published_version}
                  </span>
                </div>
                <span className="text-slate-400">
                  Updated: {new Date(activeEntry.updated_at).toLocaleString()}
                </span>
              </div>
            )}

            {/* Dynamic Schema-Driven Fields */}
            <div className="grid grid-cols-1 gap-5">
              {contentType.fields?.map((field) => (
                <div key={field.field_key} className="space-y-1.5">
                  <div className="flex items-center justify-between">
                    <label className="text-xs font-semibold text-slate-700 tracking-wide uppercase">
                      {field.name}
                      {field.configuration?.required && <span className="text-rose-500 ml-1">*</span>}
                    </label>
                    <span className="text-[10px] font-mono text-slate-400">{field.field_type}</span>
                  </div>
                  {renderFieldInput(field.field_key, field.field_type, field.configuration)}
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      ) : (
        /* COLLECTION TYPE WORKFLOW: Table List & Dynamic Modal Editor */
        <>
          <div className="flex items-center justify-between gap-4">
            <div className="relative flex-1 max-w-sm">
              <Search className="w-4 h-4 absolute left-3 top-3 text-slate-400" />
              <input
                type="text"
                placeholder="Search entries by content or ID..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full pl-9 pr-4 py-2 bg-white border border-slate-200 rounded-lg text-xs placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-sky-500"
              />
            </div>
            <div className="text-xs text-slate-500 font-medium">
              Showing {filteredEntries.length} of {totalEntries} entries
            </div>
          </div>

          <Card>
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse text-xs">
                <thead>
                  <tr className="bg-slate-50/75 border-b border-slate-100 text-slate-500 font-semibold uppercase tracking-wider text-[10px]">
                    <th className="px-6 py-3">ID / Reference</th>
                    <th className="px-6 py-3">Key Preview</th>
                    <th className="px-6 py-3">Status</th>
                    <th className="px-6 py-3">Draft / Live Version</th>
                    <th className="px-6 py-3">Last Modified</th>
                    <th className="px-6 py-3 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 text-slate-700">
                  {isLoading ? (
                    <tr>
                      <td colSpan={6} className="px-6 py-8 text-center text-slate-400">
                        Loading entries...
                      </td>
                    </tr>
                  ) : filteredEntries.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="px-6 py-10 text-center text-slate-400">
                        No entries found. Click "New Entry" above to create one.
                      </td>
                    </tr>
                  ) : (
                    filteredEntries.map((entry) => {
                      // Grab first text key for preview
                      const firstField = contentType.fields?.[0]?.field_key;
                      const previewVal =
                        firstField && entry.draft_data[firstField]
                          ? String(entry.draft_data[firstField])
                          : entry.slug || 'Untitled entry';

                      return (
                        <tr key={entry.id} className="hover:bg-slate-50/50">
                          <td className="px-6 py-3.5 font-mono text-slate-500 text-[11px] truncate max-w-[120px]">
                            {entry.id}
                          </td>
                          <td className="px-6 py-3.5 font-bold text-slate-900 truncate max-w-xs">
                            {previewVal}
                          </td>
                          <td className="px-6 py-3.5">
                            <Badge variant={entry.status}>{entry.status}</Badge>
                          </td>
                          <td className="px-6 py-3.5 font-mono text-slate-600">
                            v{entry.draft_version} / v{entry.published_version}
                          </td>
                          <td className="px-6 py-3.5 text-slate-500">
                            {new Date(entry.updated_at).toLocaleDateString()}
                          </td>
                          <td className="px-6 py-3.5 text-right space-x-1.5 whitespace-nowrap">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleOpenVersions(entry)}
                              title="View Version Timeline"
                            >
                              <History className="w-3.5 h-3.5 text-slate-500" />
                            </Button>

                            {entry.status === 'published' ? (
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => handleUnpublish(entry)}
                                title="Unpublish from live site"
                              >
                                <EyeOff className="w-3.5 h-3.5 text-amber-600" />
                              </Button>
                            ) : (
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => handlePublish(entry)}
                                title="Publish to live site"
                              >
                                <Send className="w-3.5 h-3.5 text-emerald-600" />
                              </Button>
                            )}

                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => handleEditEntry(entry)}
                            >
                              <FileEdit className="w-3.5 h-3.5" />
                              <span>Edit</span>
                            </Button>
                          </td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>
          </Card>

          {/* Edit Entry Modal */}
          <Modal
            isOpen={isEditorOpen}
            onClose={() => setIsEditorOpen(false)}
            title={activeEntry ? `Edit Entry (v${activeEntry.draft_version})` : 'New Entry'}
            description={`Manage draft data for ${contentType.name}`}
            maxWidth="xl"
          >
            <div className="space-y-4">
              {editorError && (
                <div className="p-3.5 rounded-lg bg-rose-50 border border-rose-200 flex items-center gap-2 text-xs text-rose-700">
                  <AlertCircle className="w-4 h-4 shrink-0" />
                  <span>{editorError}</span>
                </div>
              )}

              <div className="space-y-4 max-h-[60vh] overflow-y-auto pr-1">
                {contentType.fields?.map((field) => (
                  <div key={field.field_key} className="space-y-1.5">
                    <div className="flex items-center justify-between">
                      <label className="text-xs font-semibold text-slate-700 tracking-wide uppercase">
                        {field.name}
                        {field.configuration?.required && <span className="text-rose-500 ml-1">*</span>}
                      </label>
                      <span className="text-[10px] font-mono text-slate-400">{field.field_type}</span>
                    </div>
                    {renderFieldInput(field.field_key, field.field_type, field.configuration)}
                  </div>
                ))}
              </div>

              <div className="flex justify-end gap-2.5 pt-4 border-t border-slate-100">
                <Button
                  variant="outline"
                  onClick={() => setIsEditorOpen(false)}
                >
                  Cancel
                </Button>
                <Button
                  variant="primary"
                  onClick={handleSaveDraft}
                  isLoading={isSaving}
                >
                  Save Draft
                </Button>
              </div>
            </div>
          </Modal>
        </>
      )}

      {/* Version History Modal */}
      <Modal
        isOpen={isVersionModalOpen}
        onClose={() => setIsVersionModalOpen(false)}
        title="Version Timeline & History"
        description="View past immutable snapshots and restore previous revisions"
        maxWidth="lg"
      >
        <div className="space-y-4">
          {isLoadingVersions ? (
            <div className="py-8 text-center text-slate-400 text-xs">Loading versions...</div>
          ) : versionHistory.length === 0 ? (
            <div className="py-8 text-center text-slate-400 text-xs italic">
              No previous version snapshots recorded for this entry yet.
            </div>
          ) : (
            <div className="space-y-3 max-h-[60vh] overflow-y-auto">
              {versionHistory.map((ver) => (
                <div
                  key={ver.id}
                  className="p-4 rounded-xl border border-slate-200 bg-slate-50/50 flex items-start justify-between gap-4"
                >
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="font-bold text-xs text-slate-900">
                        Version {ver.version_number}
                      </span>
                      <span className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-sky-50 text-sky-700 border border-sky-100 uppercase">
                        {ver.action}
                      </span>
                    </div>
                    <div className="flex items-center gap-2 text-[11px] text-slate-500">
                      <Clock className="w-3 h-3" />
                      <span>{new Date(ver.created_at).toLocaleString()}</span>
                      {ver.changed_by && <span>• by {ver.changed_by}</span>}
                    </div>
                  </div>

                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handleRestoreVersion(ver.version_number)}
                  >
                    <RotateCcw className="w-3 h-3" />
                    <span>Restore</span>
                  </Button>
                </div>
              ))}
            </div>
          )}
        </div>
      </Modal>
    </div>
  );
};
