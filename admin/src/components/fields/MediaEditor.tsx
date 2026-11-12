import { useEffect, useMemo, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  File as FileIcon,
  FileAudio,
  Film,
  ImagePlus,
  RefreshCw,
  Trash2,
} from 'lucide-react';
import { mediaApi } from '../../api/endpoints';
import type { MediaAsset } from '../../types';
import { isVideoMediaMime, MediaPickerModal } from '../media/MediaPickerModal';
import { configFor, moveItem } from './contracts';
import type { FieldEditorProps } from './contracts';

function formatBytes(bytes?: number): string {
  if (!bytes || !Number.isFinite(bytes)) return '';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function MediaEditor({
  field,
  value,
  onChange,
  disabled,
  id,
}: FieldEditorProps & { id: string }) {
  const config = configFor(field);
  const multiple = Boolean(config.multiple);
  const allowedTypes = useMemo(
    () => (Array.isArray(config.allowed_types) ? config.allowed_types : []),
    [config.allowed_types],
  );
  const selected = useMemo(
    () =>
      Array.isArray(value)
        ? value.map(String)
        : typeof value === 'string' && value
          ? [value]
          : [],
    [value],
  );

  const [pickerOpen, setPickerOpen] = useState(false);
  const [assetsById, setAssetsById] = useState<Record<string, MediaAsset>>({});

  useEffect(() => {
    let active = true;
    const missing = selected.filter((assetId) => !assetsById[assetId]);
    if (!missing.length) return;
    Promise.all(
      missing.map((assetId) =>
        mediaApi
          .get(assetId)
          .then((res) => res.data)
          .catch(() => null),
      ),
    ).then((loaded) => {
      if (!active) return;
      const updates: Record<string, MediaAsset> = {};
      for (const item of loaded) {
        if (item) updates[item.id] = item;
      }
      if (Object.keys(updates).length > 0) {
        setAssetsById((prev) => ({ ...prev, ...updates }));
      }
    });
    return () => {
      active = false;
    };
  }, [selected, assetsById]);

  const handleConfirm = (picked: MediaAsset[]) => {
    if (!picked.length) return;
    setAssetsById((prev) => {
      const next = { ...prev };
      for (const a of picked) next[a.id] = a;
      return next;
    });
    if (multiple) {
      onChange(picked.map((a) => a.id));
    } else {
      onChange(picked[0].id);
    }
  };

  const removeAsset = (assetId: string) => {
    const next = selected.filter((idVal) => idVal !== assetId);
    onChange(multiple ? next : null);
  };

  return (
    <div className="space-y-3">
      {selected.length > 0 ? (
        <div className={multiple ? 'grid gap-2.5 sm:grid-cols-2' : 'space-y-2'}>
          {selected.map((assetId, index) => {
            const asset = assetsById[assetId];
            const isImage = asset?.mime_type?.startsWith('image/');
            const Icon = isVideoMediaMime(asset?.mime_type)
              ? Film
              : asset?.mime_type?.startsWith('audio/')
                ? FileAudio
                : FileIcon;

            return (
              <div
                key={assetId}
                className="flex items-center gap-3 rounded-xl border border-slate-200 bg-slate-50/70 p-2.5 text-xs"
              >
                {isImage && asset ? (
                  <img
                    src={asset.url}
                    alt={asset.metadata?.alt_text || asset.original_name}
                    className="h-14 w-14 shrink-0 rounded-lg border border-slate-200 object-cover bg-white"
                  />
                ) : (
                  <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-400">
                    <Icon className="h-5 w-5" />
                  </div>
                )}

                <div className="min-w-0 flex-1">
                  <div className="truncate font-semibold text-slate-800">
                    {asset?.original_name || assetId}
                  </div>
                  <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[11px] text-slate-500">
                    {asset?.mime_type && (
                      <span className="rounded bg-slate-200/70 px-1.5 py-0 font-mono uppercase">
                        {asset.mime_type.split('/')[1] || asset.mime_type}
                      </span>
                    )}
                    {asset?.metadata?.width && asset?.metadata?.height && (
                      <span>
                        {asset.metadata.width}×{asset.metadata.height}
                      </span>
                    )}
                    {asset?.file_size ? <span>{formatBytes(asset.file_size)}</span> : null}
                  </div>
                </div>

                <div className="flex shrink-0 items-center gap-1">
                  {multiple && (
                    <>
                      <button
                        type="button"
                        disabled={disabled || index === 0}
                        aria-label={`Move media ${index + 1} up`}
                        onClick={() => onChange(moveItem(selected, index, index - 1))}
                        className="rounded-md border border-slate-200 bg-white p-1 text-slate-500 hover:bg-slate-100 disabled:opacity-30 cursor-pointer"
                      >
                        <ArrowUp className="h-3.5 w-3.5" />
                      </button>
                      <button
                        type="button"
                        disabled={disabled || index === selected.length - 1}
                        aria-label={`Move media ${index + 1} down`}
                        onClick={() => onChange(moveItem(selected, index, index + 1))}
                        className="rounded-md border border-slate-200 bg-white p-1 text-slate-500 hover:bg-slate-100 disabled:opacity-30 cursor-pointer"
                      >
                        <ArrowDown className="h-3.5 w-3.5" />
                      </button>
                    </>
                  )}
                  <button
                    type="button"
                    disabled={disabled}
                    aria-label={`Remove ${asset?.original_name || assetId}`}
                    onClick={() => removeAsset(assetId)}
                    className="rounded-md border border-slate-200 bg-white p-1 text-slate-400 hover:border-rose-200 hover:bg-rose-50 hover:text-rose-600 disabled:opacity-40 cursor-pointer"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      ) : null}

      <button
        id={id}
        type="button"
        disabled={disabled}
        onClick={() => setPickerOpen(true)}
        className={`flex w-full items-center justify-center gap-2 rounded-xl border border-dashed px-4 py-3 text-xs font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50 cursor-pointer ${
          selected.length > 0
            ? 'border-slate-300 bg-white text-slate-700 hover:border-sky-400 hover:bg-sky-50/40'
            : 'border-slate-300 bg-slate-50/70 py-6 text-slate-600 hover:border-sky-400 hover:bg-sky-50/40 hover:text-sky-700'
        }`}
      >
        {selected.length > 0 ? (
          <>
            <RefreshCw className="h-3.5 w-3.5" />
            {multiple ? 'Add or reorder assets' : 'Replace asset'}
          </>
        ) : (
          <>
            <ImagePlus className="h-4 w-4 text-sky-600" />
            Click to select {multiple ? 'assets' : 'an asset'} from Media Library
          </>
        )}
      </button>

      <MediaPickerModal
        isOpen={pickerOpen}
        onClose={() => setPickerOpen(false)}
        multiple={multiple}
        allowedTypes={allowedTypes}
        initialSelectedIds={selected}
        onConfirm={handleConfirm}
      />
    </div>
  );
}
