import React, { useCallback, useEffect, useState } from 'react';
import { Eye, Mail, Pencil, Save } from 'lucide-react';
import { emailTemplatesApi } from '../../api/endpoints';
import { useToast } from '../../components/ui/Toast';
import { Alert } from '../../components/ui/Alert';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';
import { Card, CardContent, CardHeader } from '../../components/ui/Card';
import { Input } from '../../components/ui/Input';
import { SettingsCardSkeleton } from '../../components/ui/Skeleton';
import { Switch } from '../../components/ui/Switch';
import type { EmailTemplate, EmailTemplatePreview, UpdateEmailTemplateInput } from '../../types';

export interface EmailTemplatesViewProps {
  canEdit: boolean;
}

export const EmailTemplatesView: React.FC<EmailTemplatesViewProps> = ({ canEdit }) => {
  const toast = useToast();
  const [templates, setTemplates] = useState<EmailTemplate[]>([]);
  const [selectedKey, setSelectedKey] = useState<string>('');
  const [draft, setDraft] = useState<UpdateEmailTemplateInput | null>(null);
  const [activeTab, setActiveTab] = useState<'edit' | 'preview'>('edit');
  const [preview, setPreview] = useState<EmailTemplatePreview | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [isPreviewLoading, setIsPreviewLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadTemplates = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await emailTemplatesApi.list();
      const items = res.data || [];
      setTemplates(items);
      if (items.length > 0) {
        const current = items.find((t) => t.template_key === selectedKey) || items[0];
        setSelectedKey(current.template_key);
        setDraft({
          name: current.name,
          subject: current.subject,
          from_name: current.from_name,
          from_email: current.from_email,
          reply_to: current.reply_to,
          body_html: current.body_html,
          body_text: current.body_text,
          enabled: current.enabled,
        });
      }
    } catch (err: any) {
      setError(err?.message || 'Failed to load email templates.');
    } finally {
      setIsLoading(false);
    }
  }, [selectedKey]);

  useEffect(() => {
    void loadTemplates();
  }, [loadTemplates]);

  const selectedTemplate = templates.find((t) => t.template_key === selectedKey) || null;

  const selectTemplate = (tmpl: EmailTemplate) => {
    setSelectedKey(tmpl.template_key);
    setDraft({
      name: tmpl.name,
      subject: tmpl.subject,
      from_name: tmpl.from_name,
      from_email: tmpl.from_email,
      reply_to: tmpl.reply_to,
      body_html: tmpl.body_html,
      body_text: tmpl.body_text,
      enabled: tmpl.enabled,
    });
    setActiveTab('edit');
    setPreview(null);
  };

  const handleOpenPreview = async () => {
    if (!selectedKey || !draft) return;
    setActiveTab('preview');
    setIsPreviewLoading(true);
    try {
      const res = await emailTemplatesApi.preview(selectedKey, draft);
      setPreview(res.data);
    } catch (err: any) {
      setError(err?.message || 'Failed to render template preview.');
    } finally {
      setIsPreviewLoading(false);
    }
  };

  const handleSave = async () => {
    if (!selectedKey || !draft) return;
    setIsSaving(true);
    setError(null);
    try {
      const res = await emailTemplatesApi.update(selectedKey, draft);
      setTemplates((prev) =>
        prev.map((item) => (item.template_key === selectedKey ? res.data : item)),
      );
      setDraft({
        name: res.data.name,
        subject: res.data.subject,
        from_name: res.data.from_name,
        from_email: res.data.from_email,
        reply_to: res.data.reply_to,
        body_html: res.data.body_html,
        body_text: res.data.body_text,
        enabled: res.data.enabled,
      });
      toast.success('Template updated', `${res.data.name} saved and sanitized.`);
    } catch (err: any) {
      setError(err?.message || 'Failed to save email template.');
    } finally {
      setIsSaving(false);
    }
  };

  if (isLoading && templates.length === 0) {
    return <SettingsCardSkeleton rows={5} />;
  }

  return (
    <div className="space-y-4">
      {error && (
        <Alert variant="error" title="Template error">
          {error}
        </Alert>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-12 gap-5 items-start">
        <div className="lg:col-span-4 space-y-2">
          <Card>
            <CardHeader
              title="Transactional Templates"
              description="Customize subjects, sender overrides, HTML body, and plain text fallback."
            />
            <CardContent className="p-2 space-y-1">
              {templates.map((t) => {
                const isSelected = t.template_key === selectedKey;
                return (
                  <button
                    key={t.template_key}
                    type="button"
                    onClick={() => selectTemplate(t)}
                    className={`w-full text-left px-3 py-2.5 rounded-md border transition-colors ${
                      isSelected
                        ? 'bg-indigo-50 border-indigo-200 text-indigo-950'
                        : 'bg-white border-transparent hover:bg-slate-50 text-slate-700'
                    }`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="inline-flex items-center gap-1.5 text-xs font-semibold">
                        <Mail className="w-3.5 h-3.5 text-indigo-500" />
                        {t.name}
                      </span>
                      <Badge variant={t.enabled ? 'success' : 'neutral'}>
                        {t.enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    </div>
                    <p className="text-[11px] font-mono text-slate-500 mt-1">{t.template_key}</p>
                  </button>
                );
              })}
            </CardContent>
          </Card>
        </div>

        <div className="lg:col-span-8">
          {selectedTemplate && draft && (
            <Card>
              <CardHeader
                title={selectedTemplate.name}
                description={`Template key: ${selectedTemplate.template_key}`}
                action={
                  <div className="flex items-center gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant={activeTab === 'edit' ? 'primary' : 'secondary'}
                      onClick={() => setActiveTab('edit')}
                    >
                      <Pencil className="w-3.5 h-3.5" /> Edit
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant={activeTab === 'preview' ? 'primary' : 'secondary'}
                      onClick={() => void handleOpenPreview()}
                    >
                      <Eye className="w-3.5 h-3.5" /> Preview
                    </Button>
                    {canEdit && (
                      <Button
                        type="button"
                        size="sm"
                        disabled={isSaving}
                        onClick={() => void handleSave()}
                      >
                        <Save className="w-3.5 h-3.5" /> {isSaving ? 'Saving...' : 'Save Template'}
                      </Button>
                    )}
                  </div>
                }
              />
              <CardContent className="space-y-4">
                {activeTab === 'edit' ? (
                  <>
                    <div className="flex items-center justify-between gap-4 border-b border-slate-100 pb-3">
                      <div>
                        <p className="text-xs font-semibold text-slate-800">Template active</p>
                        <p className="text-[11px] text-slate-500">
                          When disabled, sends referencing this key are blocked.
                        </p>
                      </div>
                      <Switch
                        checked={draft.enabled}
                        disabled={!canEdit}
                        onChange={(checked) =>
                          setDraft((prev) => (prev ? { ...prev, enabled: checked } : prev))
                        }
                      />
                    </div>

                    <Input
                      label="Subject line"
                      value={draft.subject}
                      disabled={!canEdit}
                      onChange={(e) =>
                        setDraft((prev) => (prev ? { ...prev, subject: e.target.value } : prev))
                      }
                    />

                    <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                      <Input
                        label="From name override"
                        value={draft.from_name}
                        disabled={!canEdit}
                        placeholder="Inherit global default"
                        onChange={(e) =>
                          setDraft((prev) => (prev ? { ...prev, from_name: e.target.value } : prev))
                        }
                      />
                      <Input
                        label="From email override"
                        type="email"
                        value={draft.from_email}
                        disabled={!canEdit}
                        placeholder="Inherit global default"
                        onChange={(e) =>
                          setDraft((prev) =>
                            prev ? { ...prev, from_email: e.target.value } : prev,
                          )
                        }
                      />
                      <Input
                        label="Reply-To override"
                        type="email"
                        value={draft.reply_to}
                        disabled={!canEdit}
                        placeholder="Inherit global default"
                        onChange={(e) =>
                          setDraft((prev) => (prev ? { ...prev, reply_to: e.target.value } : prev))
                        }
                      />
                    </div>

                    {selectedTemplate.variables.length > 0 && (
                      <div className="rounded-md bg-slate-50 border border-slate-200 px-3 py-2 text-[11px] text-slate-600">
                        <span className="font-semibold text-slate-700">Available variables: </span>
                        <span className="inline-flex flex-wrap gap-1.5 ml-1">
                          {selectedTemplate.variables.map((v) => (
                            <code
                              key={v}
                              className="px-1.5 py-0.5 rounded bg-white border border-slate-200 text-indigo-700"
                            >
                              {`{{${v}}}`}
                            </code>
                          ))}
                        </span>
                      </div>
                    )}

                    <div className="space-y-1.5">
                      <label className="block text-xs font-semibold text-slate-700">
                        HTML Body (sanitized server-side)
                      </label>
                      <textarea
                        rows={7}
                        disabled={!canEdit}
                        value={draft.body_html}
                        onChange={(e) =>
                          setDraft((prev) => (prev ? { ...prev, body_html: e.target.value } : prev))
                        }
                        className="w-full rounded-md border border-slate-300 px-3 py-2 text-xs font-mono text-slate-900 focus:outline-none focus:ring-2 focus:ring-indigo-500 disabled:bg-slate-50"
                      />
                    </div>

                    <div className="space-y-1.5">
                      <label className="block text-xs font-semibold text-slate-700">
                        Plain Text Body
                      </label>
                      <textarea
                        rows={4}
                        disabled={!canEdit}
                        value={draft.body_text}
                        onChange={(e) =>
                          setDraft((prev) => (prev ? { ...prev, body_text: e.target.value } : prev))
                        }
                        className="w-full rounded-md border border-slate-300 px-3 py-2 text-xs font-mono text-slate-900 focus:outline-none focus:ring-2 focus:ring-indigo-500 disabled:bg-slate-50"
                      />
                    </div>
                  </>
                ) : (
                  <div className="space-y-4">
                    {isPreviewLoading || !preview ? (
                      <div className="py-8 text-center text-xs text-slate-500">
                        Rendering sanitized email preview...
                      </div>
                    ) : (
                      <>
                        <div className="rounded-md bg-slate-50 border border-slate-200 p-3 text-xs">
                          <span className="font-semibold text-slate-600">Rendered Subject: </span>
                          <span className="font-medium text-slate-900">{preview.subject}</span>
                        </div>
                        <div className="rounded-md border border-slate-200 overflow-hidden bg-white">
                          <iframe
                            title="Email HTML Preview"
                            sandbox=""
                            srcDoc={preview.body_html}
                            className="w-full h-64 border-0"
                          />
                        </div>
                        <div className="rounded-md bg-slate-900 text-slate-100 p-3 text-[11px] font-mono whitespace-pre-wrap">
                          {preview.body_text}
                        </div>
                      </>
                    )}
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
};
