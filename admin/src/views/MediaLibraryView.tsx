import React, { useState, useEffect, useRef } from 'react';
import {
  Upload,
  Image as ImageIcon,
  FileText,
  Search,
  Trash2,
  Copy,
  Check,
  KeyRound,
  LayoutGrid,
  List,
  AlertCircle,
  Folder as FolderIcon,
  FolderPlus,
  FolderOpen,
  ChevronRight,
  ChevronDown,
  Film,
  Move,
  Edit2,
  Play,
  Package,
  RefreshCw,
} from 'lucide-react';
import Hls from 'hls.js';
import { Card, CardContent } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Modal } from '../components/ui/Modal';
import { mediaApi, mediaFoldersApi } from '../api/endpoints';
import type { MediaAsset, MediaFolder, MediaFolderTreeNode, StorageCapabilities } from '../types';

export interface MediaLibraryViewProps {
  mediaAssets: MediaAsset[];
  onRefresh: () => void;
}

export const MediaLibraryView: React.FC<MediaLibraryViewProps> = ({
  mediaAssets,
  onRefresh,
}) => {
  const [viewMode, setViewMode] = useState<'grid' | 'table'>('grid');
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedFolderId, setSelectedFolderId] = useState<string | null>(null);

  // Folder Hierarchy Tree
  const [folderTree, setFolderTree] = useState<MediaFolderTreeNode[]>([]);
  const [expandedFolderIds, setExpandedFolderIds] = useState<Set<string>>(new Set());
  const [isTreeLoading, setIsTreeLoading] = useState(false);

  // Upload Modal (Standard & HLS)
  const [isUploadModalOpen, setIsUploadModalOpen] = useState(false);
  const [uploadTab, setUploadTab] = useState<'single' | 'hls'>('single');
  const [isUploading, setIsUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [altText, setAltText] = useState('');
  const [caption, setCaption] = useState('');
  const [hlsTitle, setHlsTitle] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);
  const hlsFileInputRef = useRef<HTMLInputElement>(null);

  // Folder CRUD Modals
  const [isCreateFolderModalOpen, setIsCreateFolderModalOpen] = useState(false);
  const [newFolderName, setNewFolderName] = useState('');
  const [newFolderParentId, setNewFolderParentId] = useState<string | null>(null);
  const [isCreatingFolder, setIsCreatingFolder] = useState(false);

  const [isRenameFolderModalOpen, setIsRenameFolderModalOpen] = useState(false);
  const [folderToRename, setFolderToRename] = useState<MediaFolder | null>(null);
  const [renamedName, setRenamedName] = useState('');
  const [isRenamingFolder, setIsRenamingFolder] = useState(false);

  // Move Asset Modal
  const [assetToMove, setAssetToMove] = useState<MediaAsset | null>(null);
  const [targetMoveFolderId, setTargetMoveFolderId] = useState<string>('root');
  const [isMovingAsset, setIsMovingAsset] = useState(false);

  // Preview Modal (Video / Image / HLS)
  const [previewAsset, setPreviewAsset] = useState<MediaAsset | null>(null);
  const videoRef = useRef<HTMLVideoElement>(null);

  // Signed URL Modal
  const [selectedAssetForSigned, setSelectedAssetForSigned] = useState<MediaAsset | null>(null);
  const [isSignedModalOpen, setIsSignedModalOpen] = useState(false);
  const [signedTTL, setSignedTTL] = useState(3600);
  const [generatedSignedURL, setGeneratedSignedURL] = useState<string | null>(null);
  const [isGeneratingSigned, setIsGeneratingSigned] = useState(false);

  // Copied state & Replace state
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [replaceTarget, setReplaceTarget] = useState<MediaAsset | null>(null);
  const [isReplacing, setIsReplacing] = useState(false);
  const replaceInputRef = useRef<HTMLInputElement>(null);
  const [storageCaps, setStorageCaps] = useState<StorageCapabilities>({
    provider: 'local',
    supports_hls: true,
    supports_signed_urls: true,
    supports_transforms: false,
    supports_folder_mapping: true,
  });

  // Load Folder Tree
  const loadFolderTree = async () => {
    setIsTreeLoading(true);
    try {
      const res = await mediaFoldersApi.tree();
      setFolderTree(res.data || []);
    } catch (err) {
      console.error('Failed to load folder tree:', err);
    } finally {
      setIsTreeLoading(false);
    }
  };

  useEffect(() => {
    loadFolderTree();
    mediaApi
      .capabilities()
      .then((res) => {
        if (res.data) setStorageCaps(res.data);
      })
      .catch(() => {});
  }, []);

  const getThumbnailSrc = (asset: MediaAsset) => {
    const thumb = asset.metadata?.thumbnails?.small || asset.metadata?.thumbnails?.medium;
    if (thumb && (thumb.startsWith('http://') || thumb.startsWith('https://'))) {
      return thumb;
    }
    return asset.url;
  };

  const triggerReplaceAsset = (asset: MediaAsset) => {
    setReplaceTarget(asset);
    if (replaceInputRef.current) {
      replaceInputRef.current.value = '';
      replaceInputRef.current.click();
    }
  };

  const handleReplaceFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file || !replaceTarget) return;
    setIsReplacing(true);
    try {
      await mediaApi.replace(replaceTarget.id, file);
      setReplaceTarget(null);
      onRefresh();
    } catch (err: any) {
      alert(err?.message || 'Failed to replace media file');
    } finally {
      setIsReplacing(false);
    }
  };

  // HLS Video Player Attachment
  useEffect(() => {
    if (!previewAsset || !videoRef.current) return;

    const isHls =
      previewAsset.mime_type === 'application/vnd.apple.mpegurl' ||
      previewAsset.metadata?.format === 'hls' ||
      previewAsset.metadata?.custom?.is_hls;

    if (!isHls) return;

    let hls: Hls | null = null;
    const video = videoRef.current;

    if (Hls.isSupported()) {
      hls = new Hls({
        enableWorker: true,
        lowLatencyMode: true,
      });
      hls.loadSource(previewAsset.url);
      hls.attachMedia(video);
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        video.play().catch(() => {});
      });
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = previewAsset.url;
      video.addEventListener('loadedmetadata', () => {
        video.play().catch(() => {});
      });
    }

    return () => {
      if (hls) {
        hls.destroy();
      }
    };
  }, [previewAsset]);

  const handleCopy = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const toggleFolderExpanded = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setExpandedFolderIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  // Create Folder
  const handleCreateFolder = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newFolderName.trim()) return;

    setIsCreatingFolder(true);
    try {
      await mediaFoldersApi.create({
        name: newFolderName.trim(),
        parent_id: newFolderParentId,
      });
      setNewFolderName('');
      setIsCreateFolderModalOpen(false);
      await loadFolderTree();
      onRefresh();
    } catch (err: any) {
      alert(`Failed to create folder: ${err.message}`);
    } finally {
      setIsCreatingFolder(false);
    }
  };

  // Rename Folder
  const handleRenameFolder = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!folderToRename || !renamedName.trim()) return;

    setIsRenamingFolder(true);
    try {
      await mediaFoldersApi.rename(folderToRename.id, renamedName.trim());
      setIsRenameFolderModalOpen(false);
      setFolderToRename(null);
      await loadFolderTree();
      onRefresh();
    } catch (err: any) {
      alert(`Failed to rename folder: ${err.message}`);
    } finally {
      setIsRenamingFolder(false);
    }
  };

  // Delete Folder
  const handleDeleteFolder = async (folder: MediaFolder) => {
    const isRecursive = window.confirm(
      `Delete folder "${folder.name}" and all of its contents recursively? Click OK for recursive delete, Cancel to abort.`
    );
    if (!isRecursive) return;

    try {
      await mediaFoldersApi.delete(folder.id, true);
      if (selectedFolderId === folder.id) {
        setSelectedFolderId(null);
      }
      await loadFolderTree();
      onRefresh();
    } catch (err: any) {
      alert(`Failed to delete folder: ${err.message}`);
    }
  };

  // Single File Upload
  const handleSingleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setUploadError(null);
    setIsUploading(true);
    try {
      await mediaApi.upload(file, altText, caption, selectedFolderId);
      setAltText('');
      setCaption('');
      setIsUploadModalOpen(false);
      if (fileInputRef.current) fileInputRef.current.value = '';
      await loadFolderTree();
      onRefresh();
    } catch (err: any) {
      setUploadError(err.message || 'File upload failed');
    } finally {
      setIsUploading(false);
    }
  };

  // HLS ZIP Upload
  const handleHlsUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setUploadError(null);
    setIsUploading(true);
    try {
      await mediaApi.uploadHls(file, hlsTitle, selectedFolderId);
      setHlsTitle('');
      setIsUploadModalOpen(false);
      if (hlsFileInputRef.current) hlsFileInputRef.current.value = '';
      await loadFolderTree();
      onRefresh();
    } catch (err: any) {
      setUploadError(err.message || 'HLS upload failed');
    } finally {
      setIsUploading(false);
    }
  };

  // Move Asset Submit
  const handleMoveAssetSubmit = async () => {
    if (!assetToMove) return;
    setIsMovingAsset(true);
    try {
      const target = targetMoveFolderId === 'root' ? null : targetMoveFolderId;
      await mediaFoldersApi.moveAsset(assetToMove.id, target);
      setAssetToMove(null);
      await loadFolderTree();
      onRefresh();
    } catch (err: any) {
      alert(`Failed to move asset: ${err.message}`);
    } finally {
      setIsMovingAsset(false);
    }
  };

  const handleDeleteAsset = async (id: string, name: string) => {
    if (!window.confirm(`Are you sure you want to delete "${name}"?`)) return;
    try {
      await mediaApi.delete(id);
      await loadFolderTree();
      onRefresh();
    } catch (err: any) {
      alert(`Failed to delete: ${err.message}`);
    }
  };

  const handleOpenSignedModal = (asset: MediaAsset) => {
    setSelectedAssetForSigned(asset);
    setGeneratedSignedURL(null);
    setIsSignedModalOpen(true);
  };

  const handleGenerateSignedURL = async () => {
    if (!selectedAssetForSigned) return;
    setIsGeneratingSigned(true);
    try {
      const res = await mediaApi.getSignedURL(selectedAssetForSigned.id, signedTTL);
      setGeneratedSignedURL(res.signed_url);
    } catch (err: any) {
      alert(`Failed to generate signed URL: ${err.message}`);
    } finally {
      setIsGeneratingSigned(false);
    }
  };

  const formatFileSize = (bytes: number) => {
    if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
    return `${(bytes / 1024).toFixed(1)} KB`;
  };

  const isImage = (mime: string) => mime.startsWith('image/');
  const isHlsAsset = (asset: MediaAsset) =>
    asset.mime_type === 'application/vnd.apple.mpegurl' ||
    asset.metadata?.format === 'hls' ||
    asset.metadata?.custom?.is_hls;

  // Find currently selected folder node
  const findFolderInTree = (nodes: MediaFolderTreeNode[], id: string): MediaFolderTreeNode | null => {
    for (const node of nodes) {
      if (node.id === id) return node;
      if (node.children) {
        const found = findFolderInTree(node.children, id);
        if (found) return found;
      }
    }
    return null;
  };

  const currentFolder = selectedFolderId ? findFolderInTree(folderTree, selectedFolderId) : null;

  // Flatten folders for move dropdown
  const flattenFolders = (nodes: MediaFolderTreeNode[], depth = 0): Array<{ id: string; name: string; depth: number }> => {
    let list: Array<{ id: string; name: string; depth: number }> = [];
    for (const n of nodes) {
      list.push({ id: n.id, name: n.name, depth });
      if (n.children && n.children.length > 0) {
        list = list.concat(flattenFolders(n.children, depth + 1));
      }
    }
    return list;
  };

  const allFolderOptions = flattenFolders(folderTree);

  // Filter assets by search and current folder selection
  const filteredAssets = mediaAssets.filter((asset) => {
    // Folder filter
    if (selectedFolderId !== null) {
      if (asset.folder_id !== selectedFolderId) return false;
    }
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return (
      asset.filename.toLowerCase().includes(q) ||
      asset.original_name.toLowerCase().includes(q) ||
      asset.mime_type.toLowerCase().includes(q)
    );
  });

  // Recursive Tree Node Renderer
  const renderFolderTreeNode = (node: MediaFolderTreeNode, depth = 0) => {
    const isExpanded = expandedFolderIds.has(node.id);
    const isSelected = selectedFolderId === node.id;
    const hasChildren = node.children && node.children.length > 0;

    return (
      <div key={node.id} className="select-none">
        <div
          onClick={() => setSelectedFolderId(node.id)}
          className={`flex items-center justify-between px-2.5 py-1.5 rounded-lg text-xs cursor-pointer transition-colors group ${
            isSelected
              ? 'bg-sky-50 text-sky-900 font-bold border border-sky-200'
              : 'text-slate-700 hover:bg-slate-100'
          }`}
          style={{ paddingLeft: `${depth * 14 + 10}px` }}
        >
          <div className="flex items-center gap-1.5 truncate">
            {hasChildren ? (
              <button
                type="button"
                onClick={(e) => toggleFolderExpanded(node.id, e)}
                className="p-0.5 rounded hover:bg-slate-200 text-slate-400 hover:text-slate-600"
              >
                {isExpanded ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
              </button>
            ) : (
              <span className="w-3.5" />
            )}
            {isSelected ? (
              <FolderOpen className="w-4 h-4 text-sky-600 shrink-0" />
            ) : (
              <FolderIcon className="w-4 h-4 text-slate-400 shrink-0" />
            )}
            <span className="truncate">{node.name}</span>
          </div>

          <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
            <span className="px-1.5 py-0.2 rounded-full text-[10px] bg-slate-200/60 text-slate-600 font-mono">
              {node.asset_count}
            </span>
            <button
              onClick={(e) => {
                e.stopPropagation();
                setFolderToRename(node);
                setRenamedName(node.name);
                setIsRenameFolderModalOpen(true);
              }}
              title="Rename folder"
              className="p-1 hover:text-sky-600"
            >
              <Edit2 className="w-3 h-3" />
            </button>
            <button
              onClick={(e) => {
                e.stopPropagation();
                handleDeleteFolder(node);
              }}
              title="Delete folder"
              className="p-1 hover:text-rose-600"
            >
              <Trash2 className="w-3 h-3" />
            </button>
          </div>
        </div>

        {hasChildren && isExpanded && (
          <div className="space-y-0.5 mt-0.5">
            {node.children.map((child) => renderFolderTreeNode(child, depth + 1))}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="space-y-6">
      {/* Top Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 tracking-tight">Media Library & Folders</h1>
          <p className="text-xs text-slate-500 mt-1">
            Production asset management with nested folders, HLS streaming video packages, auto-thumbnails, and cryptographic URLs.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            onClick={() => {
              setNewFolderParentId(selectedFolderId);
              setNewFolderName('');
              setIsCreateFolderModalOpen(true);
            }}
          >
            <FolderPlus className="w-4 h-4 text-sky-600" />
            <span>New Folder</span>
          </Button>

          <Button variant="primary" onClick={() => setIsUploadModalOpen(true)}>
            <Upload className="w-4 h-4" />
            <span>Upload Media</span>
          </Button>
        </div>
      </div>

      {/* Main Grid: Folder Sidebar + Media Content */}
      <div className="grid grid-cols-1 lg:grid-cols-4 gap-6 items-start">
        {/* Left Sidebar: Folder Tree */}
        <div className="lg:col-span-1 bg-white p-4 rounded-xl border border-slate-200 space-y-4">
          <div className="flex items-center justify-between pb-2 border-b border-slate-100">
            <h2 className="text-xs font-bold text-slate-800 uppercase tracking-wider flex items-center gap-2">
              <FolderIcon className="w-4 h-4 text-sky-600" />
              <span>Folders</span>
            </h2>
            <button
              onClick={() => {
                setNewFolderParentId(null);
                setNewFolderName('');
                setIsCreateFolderModalOpen(true);
              }}
              title="Add Root Folder"
              className="text-xs text-sky-600 hover:text-sky-700 font-semibold cursor-pointer"
            >
              + Add
            </button>
          </div>

          <div className="space-y-1">
            {/* Root "All Media" item */}
            <div
              onClick={() => setSelectedFolderId(null)}
              className={`flex items-center justify-between px-3 py-2 rounded-lg text-xs cursor-pointer transition-colors ${
                selectedFolderId === null
                  ? 'bg-sky-50 text-sky-900 font-bold border border-sky-200'
                  : 'text-slate-700 hover:bg-slate-100'
              }`}
            >
              <div className="flex items-center gap-2">
                <LayoutGrid className="w-4 h-4 text-slate-400" />
                <span>All Media</span>
              </div>
              <span className="px-2 py-0.5 rounded-full text-[10px] bg-slate-200/70 text-slate-700 font-mono">
                {mediaAssets.length}
              </span>
            </div>

            {/* Tree nodes */}
            {isTreeLoading ? (
              <p className="text-xs text-slate-400 p-3 text-center">Loading folders...</p>
            ) : folderTree.length === 0 ? (
              <p className="text-xs text-slate-400 p-3 text-center">No folders created yet</p>
            ) : (
              <div className="space-y-0.5 pt-1">
                {folderTree.map((node) => renderFolderTreeNode(node, 0))}
              </div>
            )}
          </div>
        </div>

        {/* Right Area: Breadcrumbs, Search, Assets Grid */}
        <div className="lg:col-span-3 space-y-4">
          {/* Breadcrumb Navigation */}
          <div className="flex items-center gap-2 text-xs text-slate-600 bg-white px-4 py-2.5 rounded-lg border border-slate-200">
            <button
              onClick={() => setSelectedFolderId(null)}
              className="hover:text-sky-600 font-medium cursor-pointer"
            >
              All Media
            </button>
            {currentFolder && (
              <>
                <ChevronRight className="w-3.5 h-3.5 text-slate-400" />
                <span className="font-semibold text-slate-900">{currentFolder.path}</span>
              </>
            )}
          </div>

          {/* Search & View Controls */}
          <div className="flex items-center justify-between gap-4">
            <div className="relative flex-1 max-w-sm">
              <Search className="w-4 h-4 absolute left-3 top-3 text-slate-400" />
              <input
                type="text"
                placeholder="Search assets by filename or format..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full pl-9 pr-4 py-2 bg-white border border-slate-200 rounded-lg text-xs placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-sky-500"
              />
            </div>

            <div className="flex items-center gap-1 bg-white p-1 rounded-lg border border-slate-200">
              <button
                onClick={() => setViewMode('grid')}
                className={`p-1.5 rounded text-xs transition-colors cursor-pointer ${
                  viewMode === 'grid'
                    ? 'bg-slate-100 text-slate-900 font-semibold'
                    : 'text-slate-400 hover:text-slate-700'
                }`}
              >
                <LayoutGrid className="w-4 h-4" />
              </button>
              <button
                onClick={() => setViewMode('table')}
                className={`p-1.5 rounded text-xs transition-colors cursor-pointer ${
                  viewMode === 'table'
                    ? 'bg-slate-100 text-slate-900 font-semibold'
                    : 'text-slate-400 hover:text-slate-700'
                }`}
              >
                <List className="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* Assets Grid / Table */}
          {filteredAssets.length === 0 ? (
            <Card>
              <CardContent className="p-12 text-center">
                <ImageIcon className="w-12 h-12 text-slate-300 mx-auto mb-3" />
                <h3 className="text-base font-bold text-slate-800">No media assets in this view</h3>
                <p className="text-xs text-slate-500 mt-1 max-w-sm mx-auto">
                  Upload images, videos, documents, or full HLS video packages (.zip) into this directory.
                </p>
                <Button
                  variant="primary"
                  size="sm"
                  className="mt-4"
                  onClick={() => setIsUploadModalOpen(true)}
                >
                  Upload Media
                </Button>
              </CardContent>
            </Card>
          ) : viewMode === 'grid' ? (
            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-5">
              {filteredAssets.map((asset) => {
                const isHls = isHlsAsset(asset);

                return (
                  <Card key={asset.id} className="group relative flex flex-col justify-between overflow-hidden">
                    <div>
                      {/* Preview Thumbnail */}
                      <div
                        onClick={() => setPreviewAsset(asset)}
                        className="h-44 bg-slate-950 flex items-center justify-center overflow-hidden relative cursor-pointer group-hover:opacity-95"
                      >
                        {isImage(asset.mime_type) ? (
                          <img
                            src={getThumbnailSrc(asset)}
                            alt={asset.metadata?.alt_text || asset.original_name}
                            className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
                            onError={(e) => {
                              (e.target as HTMLElement).style.display = 'none';
                            }}
                          />
                        ) : isHls ? (
                          <div className="flex flex-col items-center justify-center text-sky-400 space-y-1">
                            <Film className="w-10 h-10" />
                            <span className="text-[10px] font-mono font-bold tracking-widest uppercase bg-sky-950/80 px-2 py-0.5 rounded border border-sky-800">
                              HLS Stream
                            </span>
                            <div className="absolute inset-0 flex items-center justify-center bg-black/30 group-hover:bg-black/10 transition-colors">
                              <Play className="w-8 h-8 text-white fill-white/80 drop-shadow" />
                            </div>
                          </div>
                        ) : asset.mime_type.startsWith('video/') ? (
                          <div className="flex flex-col items-center justify-center text-indigo-300 space-y-1">
                            <Film className="w-10 h-10" />
                            <span className="text-[10px] font-mono font-bold tracking-wider uppercase bg-indigo-950/80 px-2 py-0.5 rounded border border-indigo-800">
                              Video
                            </span>
                          </div>
                        ) : (
                          <FileText className="w-12 h-12 text-slate-400" />
                        )}

                        {asset.storage_provider && (
                          <span className="absolute top-2 left-2 px-1.5 py-0.5 rounded text-[10px] font-mono font-semibold bg-indigo-900/85 text-indigo-100 backdrop-blur-xs uppercase">
                            {asset.storage_provider}
                          </span>
                        )}
                        <span className="absolute top-2 right-2 px-1.5 py-0.5 rounded text-[10px] font-mono font-semibold bg-slate-900/80 text-white backdrop-blur-xs">
                          {formatFileSize(asset.file_size)}
                        </span>
                      </div>

                      {/* Info */}
                      <div className="p-4 space-y-2">
                        <h4 className="text-xs font-bold text-slate-900 truncate" title={asset.original_name}>
                          {asset.original_name}
                        </h4>
                        <div className="flex items-center justify-between text-[11px] text-slate-500">
                          <span className="font-mono truncate max-w-[130px]">{asset.mime_type}</span>
                          {asset.metadata?.width && asset.metadata?.height && (
                            <span>
                              {asset.metadata.width}×{asset.metadata.height}
                            </span>
                          )}
                          {isHls && (
                            <span className="text-sky-600 font-semibold font-mono text-[10px]">
                              {asset.metadata?.custom?.total_files || 'HLS'} files
                            </span>
                          )}
                        </div>
                      </div>
                    </div>

                    {/* Actions */}
                    <div className="p-3 bg-slate-50 border-t border-slate-100 flex items-center justify-between gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleCopy(asset.url, asset.id)}
                        title="Copy Asset URL"
                      >
                        {copiedId === asset.id ? (
                          <Check className="w-3.5 h-3.5 text-emerald-600" />
                        ) : (
                          <Copy className="w-3.5 h-3.5" />
                        )}
                        <span className="text-[11px]">{copiedId === asset.id ? 'Copied' : 'Copy'}</span>
                      </Button>

                      <div className="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={isReplacing}
                          onClick={() => triggerReplaceAsset(asset)}
                          title="Replace Asset File"
                        >
                          <RefreshCw className="w-3.5 h-3.5 text-slate-600" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            setAssetToMove(asset);
                            setTargetMoveFolderId(asset.folder_id || 'root');
                          }}
                          title="Move to Folder"
                        >
                          <Move className="w-3.5 h-3.5 text-slate-600" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleOpenSignedModal(asset)}
                          title="Generate Signed URL"
                        >
                          <KeyRound className="w-3.5 h-3.5 text-sky-600" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleDeleteAsset(asset.id, asset.original_name)}
                          title="Delete Asset"
                        >
                          <Trash2 className="w-3.5 h-3.5 text-rose-600" />
                        </Button>
                      </div>
                    </div>
                  </Card>
                );
              })}
            </div>
          ) : (
            /* Table View */
            <Card>
              <div className="overflow-x-auto">
                <table className="w-full text-left border-collapse text-xs">
                  <thead>
                    <tr className="bg-slate-50/75 border-b border-slate-100 text-slate-500 font-semibold uppercase tracking-wider text-[10px]">
                      <th className="px-6 py-3">Asset</th>
                      <th className="px-6 py-3">MIME Type</th>
                      <th className="px-6 py-3">Size</th>
                      <th className="px-6 py-3">Storage Key</th>
                      <th className="px-6 py-3 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100 text-slate-700">
                    {filteredAssets.map((asset) => (
                      <tr key={asset.id} className="hover:bg-slate-50/50">
                        <td className="px-6 py-3.5">
                          <div
                            onClick={() => setPreviewAsset(asset)}
                            className="flex items-center gap-3 cursor-pointer group"
                          >
                            <div className="w-9 h-9 rounded-lg bg-slate-100 overflow-hidden shrink-0 flex items-center justify-center">
                              {isImage(asset.mime_type) ? (
                                <img src={getThumbnailSrc(asset)} alt="" className="w-full h-full object-cover" />
                              ) : isHlsAsset(asset) ? (
                                <Film className="w-4 h-4 text-sky-600" />
                              ) : (
                                <FileText className="w-4 h-4 text-slate-400" />
                              )}
                            </div>
                            <div className="truncate max-w-xs">
                              <p className="font-bold text-slate-900 group-hover:text-sky-600 truncate">
                                {asset.original_name}
                              </p>
                              <p className="text-[10px] text-slate-400 font-mono truncate">{asset.filename}</p>
                            </div>
                          </div>
                        </td>
                        <td className="px-6 py-3.5 font-mono text-[11px] text-slate-500">{asset.mime_type}</td>
                        <td className="px-6 py-3.5 text-slate-600">{formatFileSize(asset.file_size)}</td>
                        <td className="px-6 py-3.5 font-mono text-[11px] text-slate-400 truncate max-w-xs">
                          {asset.storage_key}
                        </td>
                        <td className="px-6 py-3.5 text-right space-x-1 whitespace-nowrap">
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={isReplacing}
                            onClick={() => triggerReplaceAsset(asset)}
                            title="Replace Asset File"
                          >
                            <RefreshCw className="w-3.5 h-3.5 text-slate-600" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleCopy(asset.url, asset.id)}
                          >
                            {copiedId === asset.id ? (
                              <Check className="w-3.5 h-3.5 text-emerald-600" />
                            ) : (
                              <Copy className="w-3.5 h-3.5" />
                            )}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => {
                              setAssetToMove(asset);
                              setTargetMoveFolderId(asset.folder_id || 'root');
                            }}
                          >
                            <Move className="w-3.5 h-3.5 text-slate-600" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleOpenSignedModal(asset)}
                          >
                            <KeyRound className="w-3.5 h-3.5 text-sky-600" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleDeleteAsset(asset.id, asset.original_name)}
                          >
                            <Trash2 className="w-3.5 h-3.5 text-rose-600" />
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>
          )}
        </div>
      </div>

      {/* Modal: Upload Media (Standard & HLS) */}
      <Modal
        isOpen={isUploadModalOpen}
        onClose={() => setIsUploadModalOpen(false)}
        title="Upload Media Assets"
        description={
          currentFolder
            ? `Uploading directly to folder: ${currentFolder.path}`
            : 'Uploading to root media directory'
        }
        maxWidth="lg"
      >
        <div className="space-y-5">
          {/* Tabs */}
          <div className="flex border-b border-slate-200">
            <button
              onClick={() => setUploadTab('single')}
              className={`px-4 py-2 text-xs font-semibold border-b-2 cursor-pointer transition-colors ${
                uploadTab === 'single'
                  ? 'border-sky-600 text-sky-600'
                  : 'border-transparent text-slate-500 hover:text-slate-800'
              }`}
            >
              Single Asset (Image, PDF, Doc)
            </button>
            <button
              onClick={() => storageCaps.supports_hls && setUploadTab('hls')}
              disabled={!storageCaps.supports_hls}
              title={
                storageCaps.supports_hls
                  ? 'Upload HLS video package zip'
                  : `HLS zip packages are not supported on ${storageCaps.provider}`
              }
              className={`px-4 py-2 text-xs font-semibold border-b-2 transition-colors flex items-center gap-1.5 ${
                !storageCaps.supports_hls
                  ? 'border-transparent text-slate-300 cursor-not-allowed'
                  : uploadTab === 'hls'
                    ? 'border-sky-600 text-sky-600 cursor-pointer'
                    : 'border-transparent text-slate-500 hover:text-slate-800 cursor-pointer'
              }`}
            >
              <Package className="w-3.5 h-3.5" />
              <span>HLS Video Package (.zip)</span>
            </button>
          </div>

          {!storageCaps.supports_hls && (
            <div className="p-2.5 rounded-lg bg-amber-50 border border-amber-200 text-[11px] text-amber-800">
              Active storage provider (<code>{storageCaps.provider}</code>) does not support multi-file HLS zip extraction. Standard video uploads (MP4, WebM, MOV) remain supported.
            </div>
          )}

          {uploadError && (
            <div className="p-3 rounded-lg bg-rose-50 border border-rose-200 flex items-center gap-2 text-xs text-rose-700">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{uploadError}</span>
            </div>
          )}

          {uploadTab === 'single' ? (
            <div className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">Alt Text (Optional)</label>
                <input
                  type="text"
                  placeholder="Descriptive text for accessibility"
                  value={altText}
                  onChange={(e) => setAltText(e.target.value)}
                  className="w-full px-3 py-2 border border-slate-300 rounded-lg text-xs focus:ring-2 focus:ring-sky-500 outline-none"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">Caption (Optional)</label>
                <input
                  type="text"
                  placeholder="Optional media caption"
                  value={caption}
                  onChange={(e) => setCaption(e.target.value)}
                  className="w-full px-3 py-2 border border-slate-300 rounded-lg text-xs focus:ring-2 focus:ring-sky-500 outline-none"
                />
              </div>

              <input
                type="file"
                ref={fileInputRef}
                onChange={handleSingleFileUpload}
                className="hidden"
              />

              <Button
                variant="primary"
                className="w-full mt-2"
                onClick={() => fileInputRef.current?.click()}
                isLoading={isUploading}
              >
                <Upload className="w-4 h-4" />
                <span>Select & Upload File</span>
              </Button>
            </div>
          ) : (
            <div className="space-y-4">
              <div className="p-3 bg-sky-50 border border-sky-100 rounded-lg text-xs text-sky-800 space-y-1">
                <p className="font-bold">HLS Video Package Instructions:</p>
                <p>
                  Upload a <code>.zip</code> archive containing your complete HLS video stream package (master playlist <code>.m3u8</code>, variant playlists, <code>.ts/.m4s</code> video segments, and subtitles).
                </p>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-700 mb-1">Video Title / Name</label>
                <input
                  type="text"
                  placeholder="e.g. Product Demo 2026"
                  value={hlsTitle}
                  onChange={(e) => setHlsTitle(e.target.value)}
                  className="w-full px-3 py-2 border border-slate-300 rounded-lg text-xs focus:ring-2 focus:ring-sky-500 outline-none"
                />
              </div>

              <input
                type="file"
                accept=".zip"
                ref={hlsFileInputRef}
                onChange={handleHlsUpload}
                className="hidden"
              />

              <Button
                variant="primary"
                className="w-full mt-2"
                onClick={() => hlsFileInputRef.current?.click()}
                isLoading={isUploading}
              >
                <Upload className="w-4 h-4" />
                <span>Select & Upload HLS Package (.zip)</span>
              </Button>
            </div>
          )}
        </div>
      </Modal>

      {/* Modal: Create Folder */}
      <Modal
        isOpen={isCreateFolderModalOpen}
        onClose={() => setIsCreateFolderModalOpen(false)}
        title="Create New Folder"
        description="Organize your media assets into hierarchical folders"
        maxWidth="md"
      >
        <form onSubmit={handleCreateFolder} className="space-y-4">
          <div>
            <label className="block text-xs font-semibold text-slate-700 mb-1">Folder Name *</label>
            <input
              type="text"
              required
              placeholder="e.g. Articles, Product Catalog, Tutorials"
              value={newFolderName}
              onChange={(e) => setNewFolderName(e.target.value)}
              className="w-full px-3 py-2 border border-slate-300 rounded-lg text-xs focus:ring-2 focus:ring-sky-500 outline-none"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold text-slate-700 mb-1">Parent Folder</label>
            <select
              value={newFolderParentId || 'root'}
              onChange={(e) => setNewFolderParentId(e.target.value === 'root' ? null : e.target.value)}
              className="w-full px-3 py-2 border border-slate-300 rounded-lg text-xs focus:ring-2 focus:ring-sky-500 outline-none bg-white"
            >
              <option value="root">/ (Root Directory)</option>
              {allFolderOptions.map((f) => (
                <option key={f.id} value={f.id}>
                  {'\u00A0'.repeat(f.depth * 3)}📁 {f.name}
                </option>
              ))}
            </select>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" type="button" onClick={() => setIsCreateFolderModalOpen(false)}>
              Cancel
            </Button>
            <Button variant="primary" type="submit" isLoading={isCreatingFolder}>
              Create Folder
            </Button>
          </div>
        </form>
      </Modal>

      {/* Modal: Rename Folder */}
      <Modal
        isOpen={isRenameFolderModalOpen}
        onClose={() => setIsRenameFolderModalOpen(false)}
        title="Rename Folder"
        description={`Update folder name and synchronize storage prefix`}
        maxWidth="md"
      >
        <form onSubmit={handleRenameFolder} className="space-y-4">
          <div>
            <label className="block text-xs font-semibold text-slate-700 mb-1">New Folder Name *</label>
            <input
              type="text"
              required
              value={renamedName}
              onChange={(e) => setRenamedName(e.target.value)}
              className="w-full px-3 py-2 border border-slate-300 rounded-lg text-xs focus:ring-2 focus:ring-sky-500 outline-none"
            />
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" type="button" onClick={() => setIsRenameFolderModalOpen(false)}>
              Cancel
            </Button>
            <Button variant="primary" type="submit" isLoading={isRenamingFolder}>
              Save Changes
            </Button>
          </div>
        </form>
      </Modal>

      {/* Modal: Move Asset */}
      <Modal
        isOpen={assetToMove !== null}
        onClose={() => setAssetToMove(null)}
        title="Move Media Asset"
        description={`Move "${assetToMove?.original_name}" to a different folder`}
        maxWidth="md"
      >
        <div className="space-y-4">
          <div>
            <label className="block text-xs font-semibold text-slate-700 mb-1">Target Folder</label>
            <select
              value={targetMoveFolderId}
              onChange={(e) => setTargetMoveFolderId(e.target.value)}
              className="w-full px-3 py-2 border border-slate-300 rounded-lg text-xs focus:ring-2 focus:ring-sky-500 outline-none bg-white"
            >
              <option value="root">/ (Root Directory)</option>
              {allFolderOptions.map((f) => (
                <option key={f.id} value={f.id}>
                  {'\u00A0'.repeat(f.depth * 3)}📁 {f.name}
                </option>
              ))}
            </select>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" type="button" onClick={() => setAssetToMove(null)}>
              Cancel
            </Button>
            <Button variant="primary" onClick={handleMoveAssetSubmit} isLoading={isMovingAsset}>
              Move Asset
            </Button>
          </div>
        </div>
      </Modal>

      {/* Hidden input for replacing an existing asset file */}
      <input
        ref={replaceInputRef}
        type="file"
        className="hidden"
        onChange={handleReplaceFileSelect}
      />

      {/* Modal: Asset Preview (HLS Player / Image / Video) */}
      <Modal
        isOpen={previewAsset !== null}
        onClose={() => setPreviewAsset(null)}
        title={previewAsset?.original_name || 'Asset Preview'}
        description={`Provider: ${previewAsset?.storage_provider || 'local'} • MIME: ${previewAsset?.mime_type} • Size: ${previewAsset ? formatFileSize(previewAsset.file_size) : ''}`}
        maxWidth="xl"
      >
        {previewAsset && (
          <div className="space-y-4">
            {isHlsAsset(previewAsset) ? (
              <div className="space-y-3">
                <div className="bg-black rounded-lg overflow-hidden aspect-video flex items-center justify-center">
                  <video
                    ref={videoRef}
                    controls
                    className="w-full h-full object-contain"
                  />
                </div>

                {previewAsset.metadata?.custom && (
                  <div className="p-3 bg-slate-50 border border-slate-200 rounded-lg text-xs space-y-1 font-mono">
                    <p className="font-bold text-slate-700 font-sans">HLS Package Details:</p>
                    <p>Total Files: {previewAsset.metadata.custom.total_files}</p>
                    <p>Master Playlist: {previewAsset.metadata.custom.master_playlist}</p>
                  </div>
                )}
              </div>
            ) : previewAsset.mime_type.startsWith('video/') ? (
              <div className="bg-black rounded-lg overflow-hidden aspect-video flex items-center justify-center">
                <video
                  src={previewAsset.url}
                  controls
                  className="w-full h-full object-contain"
                />
              </div>
            ) : isImage(previewAsset.mime_type) ? (
              <div className="bg-slate-100 rounded-lg overflow-hidden max-h-[70vh] flex items-center justify-center p-2">
                <img
                  src={previewAsset.url}
                  alt={previewAsset.metadata?.alt_text || previewAsset.original_name}
                  className="max-h-[65vh] object-contain rounded"
                />
              </div>
            ) : (
              <div className="p-8 text-center text-slate-500">
                <FileText className="w-16 h-16 mx-auto mb-2 text-slate-300" />
                <p>Preview not available for this file type.</p>
                <a
                  href={previewAsset.url}
                  target="_blank"
                  rel="noreferrer"
                  className="text-sky-600 hover:underline text-xs mt-2 inline-block font-semibold"
                >
                  Download / Open File
                </a>
              </div>
            )}
          </div>
        )}
      </Modal>

      {/* Modal: Generate Signed URL */}
      <Modal
        isOpen={isSignedModalOpen}
        onClose={() => setIsSignedModalOpen(false)}
        title="Generate Temporary Signed URL"
        description="Create a cryptographically signed HMAC-SHA256 link with expiration"
        maxWidth="md"
      >
        <div className="space-y-4">
          <div>
            <label className="block text-xs font-semibold text-slate-700 tracking-wide uppercase mb-1.5">
              Expiration Duration (TTL)
            </label>
            <select
              value={signedTTL}
              onChange={(e) => setSignedTTL(Number(e.target.value))}
              className="w-full px-3 py-2 bg-white border border-slate-300 rounded-lg text-sm text-slate-900 focus:outline-none focus:ring-2 focus:ring-sky-500"
            >
              <option value={900}>15 Minutes</option>
              <option value={3600}>1 Hour (Default)</option>
              <option value={14400}>4 Hours</option>
              <option value={86400}>24 Hours</option>
            </select>
          </div>

          <Button
            variant="primary"
            className="w-full"
            onClick={handleGenerateSignedURL}
            isLoading={isGeneratingSigned}
          >
            <KeyRound className="w-4 h-4" />
            <span>Generate Cryptographic Signed URL</span>
          </Button>

          {generatedSignedURL && (
            <div className="p-3.5 rounded-xl bg-slate-900 text-slate-100 space-y-2 mt-4">
              <div className="flex items-center justify-between text-xs text-slate-400">
                <span className="font-semibold text-sky-400">Signed URL:</span>
                <button
                  onClick={() => handleCopy(generatedSignedURL, 'modal-signed')}
                  className="flex items-center gap-1 text-[11px] text-sky-300 hover:text-white cursor-pointer"
                >
                  {copiedId === 'modal-signed' ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
                  <span>{copiedId === 'modal-signed' ? 'Copied' : 'Copy'}</span>
                </button>
              </div>
              <p className="font-mono text-xs break-all text-slate-200 select-all">
                {generatedSignedURL}
              </p>
            </div>
          )}
        </div>
      </Modal>
    </div>
  );
};
