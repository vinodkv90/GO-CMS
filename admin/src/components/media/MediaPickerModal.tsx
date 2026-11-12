import React, { useEffect, useId, useMemo, useRef, useState } from 'react';
import {
  Check,
  ChevronDown,
  ChevronRight,
  File as FileIcon,
  FileAudio,
  Film,
  Folder,
  FolderOpen,
  Grid2x2,
  Image as ImageIcon,
  List as ListIcon,
  Search,
  Upload,
  X,
} from 'lucide-react';
import { mediaApi, mediaFoldersApi } from '../../api/endpoints';
import type { MediaAsset, MediaFolderTreeNode } from '../../types';
import { Button } from '../ui/Button';

export interface MediaPickerModalProps {
  isOpen: boolean;
  onClose: () => void;
  multiple?: boolean;
  allowedTypes?: string[];
  initialSelectedIds?: string[];
  onConfirm: (selectedAssets: MediaAsset[]) => void;
}

const HLS_MIME_TYPES = new Set([
  'application/vnd.apple.mpegurl',
  'application/x-mpegurl',
]);

export function isVideoMediaMime(mimeType?: string): boolean {
  const mime = (mimeType || '').toLowerCase().trim();
  return mime.startsWith('video/') || HLS_MIME_TYPES.has(mime);
}

export function matchesAllowedTypes(mimeType: string, allowedTypes: string[]): boolean {
  if (!allowedTypes.length) return true;
  const mime = (mimeType || '').toLowerCase().trim();
  return allowedTypes.some((raw) => {
    const rule = raw.toLowerCase().trim();
    if (!rule) return true;
    if (rule === 'image' || rule === 'images') return mime.startsWith('image/');
    if (rule === 'video' || rule === 'videos') return isVideoMediaMime(mime);
    if (rule === 'audio' || rule === 'audios') return mime.startsWith('audio/');
    if (rule === 'file' || rule === 'files') {
      return !mime.startsWith('image/') && !isVideoMediaMime(mime) && !mime.startsWith('audio/');
    }
    if (rule.endsWith('/*')) return mime.startsWith(rule.slice(0, -1));
    if (rule.endsWith('/')) return mime.startsWith(rule);
    return mime === rule || mime.startsWith(`${rule}/`);
  });
}

function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function flattenFolders(
  nodes: MediaFolderTreeNode[],
  depth = 0,
): Array<{ id: string; name: string; depth: number }> {
  const result: Array<{ id: string; name: string; depth: number }> = [];
  for (const node of nodes) {
    result.push({ id: node.id, name: node.name, depth });
    if (node.children?.length) {
      result.push(...flattenFolders(node.children, depth + 1));
    }
  }
  return result;
}

const EMPTY_STRING_ARRAY: string[] = [];

export const MediaPickerModal: React.FC<MediaPickerModalProps> = ({
  isOpen,
  onClose,
  multiple = false,
  allowedTypes = EMPTY_STRING_ARRAY,
  initialSelectedIds = EMPTY_STRING_ARRAY,
  onConfirm,
}) => {
  const [activeTab, setActiveTab] = useState<'browse' | 'upload'>('browse');
  const [viewLayout, setViewLayout] = useState<'grid' | 'list'>('grid');
  const [tree, setTree] = useState<MediaFolderTreeNode[]>([]);
  const [expandedFolders, setExpandedFolders] = useState<Record<string, boolean>>({});
  const [selectedFolderId, setSelectedFolderId] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const [assets, setAssets] = useState<MediaAsset[]>([]);
  const [loadingAssets, setLoadingAssets] = useState(false);
  const [loadError, setLoadError] = useState('');

  const allowedTypesKey = allowedTypes.join('\0');
  const stableAllowedTypes = useMemo(
    () => (allowedTypesKey ? allowedTypesKey.split('\0') : EMPTY_STRING_ARRAY),
    [allowedTypesKey],
  );
  const initialSelectedIdsKey = initialSelectedIds.join('\0');
  const stableInitialSelectedIds = useMemo(
    () =>
      initialSelectedIdsKey
        ? initialSelectedIdsKey.split('\0')
        : EMPTY_STRING_ARRAY,
    [initialSelectedIdsKey],
  );

  // Selected items preserved across searches/folders
  const [selectedAssetsMap, setSelectedAssetsMap] = useState<Record<string, MediaAsset>>({});
  const [selectedOrder, setSelectedOrder] = useState<string[]>(stableInitialSelectedIds);
  const selectedOrderRef = useRef<string[]>(selectedOrder);
  useEffect(() => {
    selectedOrderRef.current = selectedOrder;
  }, [selectedOrder]);

  // Upload state
  const [uploadMode, setUploadMode] = useState<'standard' | 'hls'>('standard');
  const [uploadFolderId, setUploadFolderId] = useState<string | null>(null);
  const [uploadAlt, setUploadAlt] = useState('');
  const [uploadCaption, setUploadCaption] = useState('');
  const [uploadTitle, setUploadTitle] = useState('');
  const folderSelectId = useId();
  const altInputId = useId();
  const captionInputId = useId();
  const hlsTitleId = useId();
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState('');

  useEffect(() => {
    if (!isOpen) return;
    let active = true;
    setSelectedOrder(stableInitialSelectedIds);
    setActiveTab('browse');
    setUploadError('');
    setLoadError('');

    mediaFoldersApi
      .tree()
      .then((res) => {
        if (active) setTree(res.data || []);
      })
      .catch(() => {
        if (active) setTree([]);
      });

    if (stableInitialSelectedIds.length > 0) {
      Promise.all(
        stableInitialSelectedIds.map((id) =>
          mediaApi
            .get(id)
            .then((r) => r.data)
            .catch(() => null),
        ),
      ).then((loaded) => {
        if (!active) return;
        const map: Record<string, MediaAsset> = {};
        for (const asset of loaded) {
          if (asset) map[asset.id] = asset;
        }
        setSelectedAssetsMap((prev) => ({ ...prev, ...map }));
      });
    }
    return () => {
      active = false;
    };
  }, [isOpen, stableInitialSelectedIds]);

  useEffect(() => {
    if (!isOpen) return;
    let active = true;
    const timer = window.setTimeout(() => {
      setLoadingAssets(true);
      setLoadError('');
      mediaApi
        .list({
          search: searchQuery || undefined,
          folder_id: selectedFolderId,
          limit: 60,
        })
        .then((res) => {
          if (!active) return;
          const compatible = (res.data || []).filter((asset) =>
            matchesAllowedTypes(asset.mime_type, stableAllowedTypes),
          );
          setAssets(compatible);
          setSelectedAssetsMap((prev) => {
            const next = { ...prev };
            for (const a of compatible) {
              if (selectedOrderRef.current.includes(a.id)) next[a.id] = a;
            }
            return next;
          });
        })
        .catch((err: unknown) => {
          if (!active) return;
          setLoadError(
            err instanceof Error ? err.message : 'Unable to load media assets.',
          );
        })
        .finally(() => {
          if (active) setLoadingAssets(false);
        });
    }, 150);

    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [isOpen, searchQuery, selectedFolderId, stableAllowedTypes]);

  useEffect(() => {
    if (!isOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !uploading) onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isOpen, uploading, onClose]);

  const flatFolders = useMemo(() => flattenFolders(tree), [tree]);

  if (!isOpen) return null;

  const toggleAssetSelection = (asset: MediaAsset) => {
    setSelectedAssetsMap((prev) => ({ ...prev, [asset.id]: asset }));
    if (multiple) {
      setSelectedOrder((prev) =>
        prev.includes(asset.id)
          ? prev.filter((id) => id !== asset.id)
          : [...prev, asset.id],
      );
    } else {
      setSelectedOrder([asset.id]);
    }
  };

  const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    if (uploadMode === 'standard' && allowedTypes.length > 0) {
      if (!matchesAllowedTypes(file.type, allowedTypes)) {
        setUploadError(
          `This field restricts uploads to: ${allowedTypes.join(', ')} (got ${file.type || 'unknown'}).`,
        );
        e.target.value = '';
        return;
      }
    }

    setUploading(true);
    setUploadError('');
    try {
      const targetFolder = uploadFolderId ?? selectedFolderId;
      const res =
        uploadMode === 'hls'
          ? await mediaApi.uploadHls(file, uploadTitle, targetFolder)
          : await mediaApi.upload(file, uploadAlt, uploadCaption, targetFolder);
      const uploaded = res.data;
      setAssets((prev) => [uploaded, ...prev.filter((a) => a.id !== uploaded.id)]);
      setSelectedAssetsMap((prev) => ({ ...prev, [uploaded.id]: uploaded }));
      if (multiple) {
        setSelectedOrder((prev) => [...new Set([...prev, uploaded.id])]);
      } else {
        setSelectedOrder([uploaded.id]);
      }
      setUploadAlt('');
      setUploadCaption('');
      setUploadTitle('');
      setActiveTab('browse');
    } catch (err: unknown) {
      setUploadError(
        err instanceof Error ? err.message : 'Failed to upload asset.',
      );
    } finally {
      setUploading(false);
      e.target.value = '';
    }
  };

  const confirmSelection = () => {
    const resolved = selectedOrder
      .map(
        (id) =>
          selectedAssetsMap[id] || assets.find((a) => a.id === id),
      )
      .filter((a): a is MediaAsset => Boolean(a));
    onConfirm(resolved);
    onClose();
  };

  const renderFolderTree = (nodes: MediaFolderTreeNode[], depth = 0) =>
    nodes.map((node) => {
      const hasChildren = node.children && node.children.length > 0;
      const isExpanded = expandedFolders[node.id] ?? true;
      const isSelected = selectedFolderId === node.id;
      return (
        <div key={node.id}>
          <div
            className={`flex items-center gap-1 rounded-lg py-1.5 pr-2 text-xs transition-colors ${
              isSelected
                ? 'bg-sky-50 font-semibold text-sky-700'
                : 'text-slate-700 hover:bg-slate-100'
            }`}
            style={{ paddingLeft: `${8 + depth * 14}px` }}
          >
            {hasChildren ? (
              <button
                type="button"
                onClick={() =>
                  setExpandedFolders((prev) => ({
                    ...prev,
                    [node.id]: !isExpanded,
                  }))
                }
                className="rounded p-0.5 text-slate-400 hover:text-slate-700 cursor-pointer"
                aria-label={`Toggle ${node.name}`}
              >
                {isExpanded ? (
                  <ChevronDown className="h-3 w-3" />
                ) : (
                  <ChevronRight className="h-3 w-3" />
                )}
              </button>
            ) : (
              <span className="w-4" />
            )}
            <button
              type="button"
              onClick={() => setSelectedFolderId(node.id)}
              className="flex min-w-0 flex-1 items-center gap-1.5 text-left cursor-pointer"
            >
              {isSelected ? (
                <FolderOpen className="h-3.5 w-3.5 shrink-0 text-sky-600" />
              ) : (
                <Folder className="h-3.5 w-3.5 shrink-0 text-slate-400" />
              )}
              <span className="truncate">{node.name}</span>
              <span className="ml-auto shrink-0 text-[10px] text-slate-400">
                {node.asset_count}
              </span>
            </button>
          </div>
          {hasChildren && isExpanded && renderFolderTree(node.children, depth + 1)}
        </div>
      );
    });

  const renderThumbnail = (asset: MediaAsset, compact = false) => {
    if (asset.mime_type.startsWith('image/')) {
      return (
        <img
          src={asset.url}
          alt={asset.metadata?.alt_text || asset.original_name}
          className={
            compact
              ? 'h-10 w-10 shrink-0 rounded object-cover'
              : 'h-28 w-full object-cover'
          }
        />
      );
    }
    const Icon = isVideoMediaMime(asset.mime_type)
      ? Film
      : asset.mime_type.startsWith('audio/')
        ? FileAudio
        : FileIcon;
    return (
      <div
        className={`flex items-center justify-center bg-slate-100 text-slate-500 ${
          compact ? 'h-10 w-10 shrink-0 rounded' : 'h-28 w-full'
        }`}
      >
        <Icon className={compact ? 'h-4 w-4' : 'h-7 w-7'} />
      </div>
    );
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 backdrop-blur-xs p-3 sm:p-6">
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Media Library Picker"
        className="flex max-h-[90vh] w-full max-w-5xl flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl"
      >
        {/* Header */}
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 px-6 py-4">
          <div className="flex items-center gap-4">
            <h2 className="text-lg font-bold text-slate-900">
              {multiple ? 'Select media assets' : 'Select media asset'}
            </h2>
            <div className="inline-flex rounded-lg bg-slate-100 p-1 text-xs" role="tablist">
              <button
                type="button"
                role="tab"
                aria-selected={activeTab === 'browse'}
                onClick={() => setActiveTab('browse')}
                className={`rounded-md px-3 py-1 font-semibold transition-colors cursor-pointer ${
                  activeTab === 'browse'
                    ? 'bg-white text-sky-700 shadow-2xs'
                    : 'text-slate-600 hover:text-slate-900'
                }`}
              >
                Browse Library
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={activeTab === 'upload'}
                onClick={() => {
                  setUploadFolderId(selectedFolderId);
                  setActiveTab('upload');
                }}
                className={`flex items-center gap-1.5 rounded-md px-3 py-1 font-semibold transition-colors cursor-pointer ${
                  activeTab === 'upload'
                    ? 'bg-white text-sky-700 shadow-2xs'
                    : 'text-slate-600 hover:text-slate-900'
                }`}
              >
                <Upload className="h-3.5 w-3.5" />
                Upload new assets
              </button>
            </div>
          </div>

          <button
            type="button"
            disabled={uploading}
            onClick={onClose}
            aria-label="Close media picker"
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 hover:text-slate-700 cursor-pointer"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body */}
        {activeTab === 'browse' ? (
          <div className="flex min-h-[420px] flex-1 overflow-hidden">
            {/* Folder Sidebar */}
            <aside className="hidden w-56 shrink-0 flex-col border-r border-slate-200 bg-slate-50/70 p-3 sm:flex">
              <div className="mb-2 px-2 text-[11px] font-bold uppercase tracking-wider text-slate-400">
                Folders
              </div>
              <div className="flex-1 space-y-0.5 overflow-y-auto">
                <button
                  type="button"
                  onClick={() => setSelectedFolderId(null)}
                  className={`flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs transition-colors cursor-pointer ${
                    selectedFolderId === null
                      ? 'bg-sky-50 font-semibold text-sky-700'
                      : 'text-slate-700 hover:bg-slate-100'
                  }`}
                >
                  <ImageIcon className="h-3.5 w-3.5 text-sky-600" />
                  <span>All Media Assets</span>
                </button>
                {renderFolderTree(tree)}
              </div>
            </aside>

            {/* Assets Browser */}
            <div className="flex flex-1 min-w-0 flex-col">
              {/* Filter Bar */}
              <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-5 py-3">
                <div className="relative flex-1 max-w-sm">
                  <Search className="pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
                  <input
                    id="media-picker-search"
                    name="mediaPickerSearch"
                    type="search"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    placeholder="Search assets by filename or alt..."
                    className="w-full rounded-lg border border-slate-200 bg-slate-50 py-1.5 pr-3 pl-8 text-xs text-slate-900 focus:border-sky-500 focus:bg-white focus:outline-none"
                  />
                </div>

                <div className="flex items-center gap-2">
                  {allowedTypes.length > 0 && (
                    <span className="rounded-full bg-slate-100 px-2.5 py-1 text-[11px] font-medium text-slate-600">
                      Filter: {allowedTypes.join(', ')}
                    </span>
                  )}
                  <div className="inline-flex rounded-lg border border-slate-200 bg-white p-0.5">
                    <button
                      type="button"
                      onClick={() => setViewLayout('grid')}
                      aria-label="Grid view"
                      className={`rounded p-1 cursor-pointer ${
                        viewLayout === 'grid'
                          ? 'bg-sky-50 text-sky-700'
                          : 'text-slate-400 hover:text-slate-700'
                      }`}
                    >
                      <Grid2x2 className="h-3.5 w-3.5" />
                    </button>
                    <button
                      type="button"
                      onClick={() => setViewLayout('list')}
                      aria-label="List view"
                      className={`rounded p-1 cursor-pointer ${
                        viewLayout === 'list'
                          ? 'bg-sky-50 text-sky-700'
                          : 'text-slate-400 hover:text-slate-700'
                      }`}
                    >
                      <ListIcon className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              </div>

              {/* Assets Grid/List */}
              <div className="flex-1 overflow-y-auto p-5">
                {loadError && (
                  <p role="alert" className="mb-3 rounded-lg bg-rose-50 p-3 text-xs text-rose-700">
                    {loadError}
                  </p>
                )}

                {loadingAssets ? (
                  <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
                    {Array.from({ length: 8 }).map((_, i) => (
                      <div
                        key={i}
                        className="h-40 animate-pulse rounded-xl border border-slate-200 bg-slate-100"
                      />
                    ))}
                  </div>
                ) : assets.length === 0 ? (
                  <div className="flex h-64 flex-col items-center justify-center text-center">
                    <ImageIcon className="mb-2 h-8 w-8 text-slate-300" />
                    <p className="text-sm font-semibold text-slate-700">
                      No matching assets found
                    </p>
                    <p className="mt-1 text-xs text-slate-500">
                      Try another folder or upload a new asset directly.
                    </p>
                    <Button
                      size="sm"
                      className="mt-4"
                      onClick={() => setActiveTab('upload')}
                    >
                      <Upload className="h-3.5 w-3.5" />
                      Upload asset
                    </Button>
                  </div>
                ) : viewLayout === 'grid' ? (
                  <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
                    {assets.map((asset) => {
                      const isSelected = selectedOrder.includes(asset.id);
                      return (
                        <button
                          key={asset.id}
                          type="button"
                          onClick={() => toggleAssetSelection(asset)}
                          onDoubleClick={() => {
                            if (!multiple) {
                              onConfirm([asset]);
                              onClose();
                            }
                          }}
                          className={`group relative flex flex-col overflow-hidden rounded-xl border text-left transition-all cursor-pointer ${
                            isSelected
                              ? 'border-sky-500 bg-sky-50/40 ring-2 ring-sky-200'
                              : 'border-slate-200 bg-white hover:border-sky-300'
                          }`}
                        >
                          <div className="relative bg-slate-100">
                            {renderThumbnail(asset)}
                            <span
                              className={`absolute top-2 right-2 flex h-5 w-5 items-center justify-center rounded-full border text-xs transition-colors ${
                                isSelected
                                  ? 'border-sky-600 bg-sky-600 text-white'
                                  : 'border-white/80 bg-white/80 text-transparent group-hover:border-slate-300'
                              }`}
                            >
                              <Check className="h-3 w-3" />
                            </span>
                          </div>
                          <div className="p-2.5">
                            <div className="truncate text-xs font-semibold text-slate-800">
                              {asset.original_name}
                            </div>
                            <div className="mt-0.5 flex items-center justify-between text-[10px] text-slate-400">
                              <span className="truncate uppercase">
                                {asset.mime_type.split('/')[1] || asset.mime_type}
                              </span>
                              <span>{formatFileSize(asset.file_size)}</span>
                            </div>
                          </div>
                        </button>
                      );
                    })}
                  </div>
                ) : (
                  <div className="divide-y divide-slate-100 rounded-xl border border-slate-200">
                    {assets.map((asset) => {
                      const isSelected = selectedOrder.includes(asset.id);
                      return (
                        <button
                          key={asset.id}
                          type="button"
                          onClick={() => toggleAssetSelection(asset)}
                          className={`flex w-full items-center gap-3 p-2.5 text-left text-xs transition-colors cursor-pointer ${
                            isSelected ? 'bg-sky-50' : 'hover:bg-slate-50'
                          }`}
                        >
                          <span
                            className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border ${
                              isSelected
                                ? 'border-sky-600 bg-sky-600 text-white'
                                : 'border-slate-300 bg-white'
                            }`}
                          >
                            {isSelected && <Check className="h-3 w-3" />}
                          </span>
                          {renderThumbnail(asset, true)}
                          <div className="min-w-0 flex-1">
                            <div className="truncate font-semibold text-slate-800">
                              {asset.original_name}
                            </div>
                            <div className="text-[11px] text-slate-400">
                              {asset.mime_type}
                              {asset.metadata?.width && asset.metadata?.height
                                ? ` • ${asset.metadata.width}×${asset.metadata.height}`
                                : ''}
                            </div>
                          </div>
                          <span className="shrink-0 text-slate-500">
                            {formatFileSize(asset.file_size)}
                          </span>
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>
          </div>
        ) : (
          /* Upload Tab */
          <div className="min-h-[420px] flex-1 overflow-y-auto p-6">
            <div className="mx-auto max-w-xl space-y-5">
              <div className="flex gap-2 rounded-lg bg-slate-100 p-1 text-xs">
                <button
                  type="button"
                  onClick={() => setUploadMode('standard')}
                  className={`flex-1 rounded-md py-1.5 font-semibold cursor-pointer ${
                    uploadMode === 'standard'
                      ? 'bg-white text-sky-700 shadow-2xs'
                      : 'text-slate-600'
                  }`}
                >
                  Standard File / Image / Video
                </button>
                <button
                  type="button"
                  onClick={() => setUploadMode('hls')}
                  className={`flex-1 rounded-md py-1.5 font-semibold cursor-pointer ${
                    uploadMode === 'hls'
                      ? 'bg-white text-sky-700 shadow-2xs'
                      : 'text-slate-600'
                  }`}
                >
                  HLS Video Package (.zip)
                </button>
              </div>

              {uploadError && (
                <div
                  role="alert"
                  className="rounded-lg border border-rose-200 bg-rose-50 px-3.5 py-2.5 text-xs text-rose-700"
                >
                  {uploadError}
                </div>
              )}

              <div>
                <label
                  htmlFor={folderSelectId}
                  className="mb-1 block text-xs font-semibold text-slate-700"
                >
                  Target folder
                </label>
                <select
                  id={folderSelectId}
                  name="target_folder"
                  value={uploadFolderId || ''}
                  onChange={(e) => setUploadFolderId(e.target.value || null)}
                  className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-800"
                >
                  <option value="">Root / All Assets</option>
                  {flatFolders.map((f) => (
                    <option key={f.id} value={f.id}>
                      {'— '.repeat(f.depth)}
                      {f.name}
                    </option>
                  ))}
                </select>
              </div>

              {uploadMode === 'standard' ? (
                <div className="grid gap-4 sm:grid-cols-2">
                  <div>
                    <label
                      htmlFor={altInputId}
                      className="mb-1 block text-xs font-semibold text-slate-700"
                    >
                      Alternative text (optional)
                    </label>
                    <input
                      id={altInputId}
                      name="alt_text"
                      value={uploadAlt}
                      onChange={(e) => setUploadAlt(e.target.value)}
                      placeholder="Describe the asset..."
                      className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
                    />
                  </div>
                  <div>
                    <label
                      htmlFor={captionInputId}
                      className="mb-1 block text-xs font-semibold text-slate-700"
                    >
                      Caption (optional)
                    </label>
                    <input
                      id={captionInputId}
                      name="caption"
                      value={uploadCaption}
                      onChange={(e) => setUploadCaption(e.target.value)}
                      placeholder="Optional caption"
                      className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
                    />
                  </div>
                </div>
              ) : (
                <div>
                  <label
                    htmlFor={hlsTitleId}
                    className="mb-1 block text-xs font-semibold text-slate-700"
                  >
                    HLS Stream Title
                  </label>
                  <input
                    id={hlsTitleId}
                    name="hls_title"
                    value={uploadTitle}
                    onChange={(e) => setUploadTitle(e.target.value)}
                    placeholder="e.g. Product Launch Demo"
                    className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm"
                  />
                </div>
              )}

              <label className="flex cursor-pointer flex-col items-center justify-center rounded-2xl border-2 border-dashed border-slate-300 bg-slate-50 px-6 py-10 text-center transition-colors hover:border-sky-400 hover:bg-sky-50/40">
                <Upload className="mb-2 h-8 w-8 text-sky-600" />
                <span className="text-sm font-semibold text-slate-800">
                  {uploading ? 'Uploading asset...' : 'Click to select a file to upload'}
                </span>
                <span className="mt-1 text-xs text-slate-500">
                  {uploadMode === 'hls'
                    ? 'Select a .zip containing an HLS playlist (.m3u8) & segments'
                    : allowedTypes.length
                      ? `Allowed types: ${allowedTypes.join(', ')}`
                      : 'Images, videos, audio, or document files'}
                </span>
                <input
                  id="media-picker-file"
                  name="mediaPickerFile"
                  type="file"
                  disabled={uploading}
                  accept={uploadMode === 'hls' ? '.zip' : undefined}
                  onChange={handleFileUpload}
                  className="sr-only"
                />
              </label>
            </div>
          </div>
        )}

        {/* Footer */}
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-200 bg-slate-50/80 px-6 py-3.5">
          <div className="text-xs text-slate-600">
            {selectedOrder.length === 0 ? (
              <span>No asset selected</span>
            ) : (
              <span className="font-semibold text-sky-700">
                {selectedOrder.length} {selectedOrder.length === 1 ? 'asset' : 'assets'} selected
              </span>
            )}
          </div>

          <div className="flex items-center gap-2">
            <Button variant="outline" disabled={uploading} onClick={onClose}>
              Cancel
            </Button>
            <Button
              disabled={uploading || selectedOrder.length === 0}
              onClick={confirmSelection}
            >
              Finish
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};

