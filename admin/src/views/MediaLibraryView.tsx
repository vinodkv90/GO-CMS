import React, { useState, useRef } from 'react';
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
  File,
} from 'lucide-react';
import { Card, CardContent } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Modal } from '../components/ui/Modal';
import { mediaApi } from '../api/endpoints';
import type { MediaAsset } from '../types';

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
  const [isUploading, setIsUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);

  // Upload fields
  const [altText, setAltText] = useState('');
  const [caption, setCaption] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Signed URL Modal
  const [selectedAsset, setSelectedAsset] = useState<MediaAsset | null>(null);
  const [isSignedModalOpen, setIsSignedModalOpen] = useState(false);
  const [signedTTL, setSignedTTL] = useState(3600); // 1 hour default
  const [generatedSignedURL, setGeneratedSignedURL] = useState<string | null>(null);
  const [isGeneratingSigned, setIsGeneratingSigned] = useState(false);

  // Copied state
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const handleCopy = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setUploadError(null);
    setIsUploading(true);
    try {
      await mediaApi.upload(file, altText, caption);
      setAltText('');
      setCaption('');
      if (fileInputRef.current) fileInputRef.current.value = '';
      onRefresh();
    } catch (err: any) {
      setUploadError(err.message || 'File upload failed');
    } finally {
      setIsUploading(false);
    }
  };

  const handleDelete = async (id: string, name: string) => {
    if (!window.confirm(`Are you sure you want to delete asset "${name}"?`)) return;
    try {
      await mediaApi.delete(id);
      onRefresh();
    } catch (err: any) {
      alert(`Failed to delete: ${err.message}`);
    }
  };

  const handleOpenSignedModal = (asset: MediaAsset) => {
    setSelectedAsset(asset);
    setGeneratedSignedURL(null);
    setIsSignedModalOpen(true);
  };

  const handleGenerateSignedURL = async () => {
    if (!selectedAsset) return;
    setIsGeneratingSigned(true);
    try {
      const res = await mediaApi.getSignedURL(selectedAsset.id, signedTTL);
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

  const filteredAssets = mediaAssets.filter((asset) => {
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return (
      asset.filename.toLowerCase().includes(q) ||
      asset.original_name.toLowerCase().includes(q) ||
      asset.mime_type.toLowerCase().includes(q)
    );
  });

  return (
    <div className="space-y-6">
      {/* Top Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 tracking-tight">Media Library</h1>
          <p className="text-xs text-slate-500 mt-1">
            Secure asset repository with auto-thumbnails, path traversal protection, and HMAC-signed URLs.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <input
            type="file"
            ref={fileInputRef}
            onChange={handleFileSelect}
            className="hidden"
          />
          <Button
            variant="primary"
            onClick={() => fileInputRef.current?.click()}
            isLoading={isUploading}
          >
            <Upload className="w-4 h-4" />
            <span>Upload New Asset</span>
          </Button>
        </div>
      </div>

      {/* Upload Error Banner */}
      {uploadError && (
        <div className="p-3.5 rounded-lg bg-rose-50 border border-rose-200 flex items-center gap-2 text-xs text-rose-700">
          <AlertCircle className="w-4 h-4 shrink-0" />
          <span>{uploadError}</span>
        </div>
      )}

      {/* Search and View Controls */}
      <div className="flex items-center justify-between gap-4">
        <div className="relative flex-1 max-w-sm">
          <Search className="w-4 h-4 absolute left-3 top-3 text-slate-400" />
          <input
            type="text"
            placeholder="Search assets by filename or mime type..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-9 pr-4 py-2 bg-white border border-slate-200 rounded-lg text-xs placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-sky-500"
          />
        </div>

        <div className="flex items-center gap-1 bg-white p-1 rounded-lg border border-slate-200">
          <button
            onClick={() => setViewMode('grid')}
            className={`p-1.5 rounded text-xs transition-colors cursor-pointer ${
              viewMode === 'grid' ? 'bg-slate-100 text-slate-900 font-semibold' : 'text-slate-400 hover:text-slate-700'
            }`}
          >
            <LayoutGrid className="w-4 h-4" />
          </button>
          <button
            onClick={() => setViewMode('table')}
            className={`p-1.5 rounded text-xs transition-colors cursor-pointer ${
              viewMode === 'table' ? 'bg-slate-100 text-slate-900 font-semibold' : 'text-slate-400 hover:text-slate-700'
            }`}
          >
            <List className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Assets Content */}
      {filteredAssets.length === 0 ? (
        <Card>
          <CardContent className="p-12 text-center">
            <ImageIcon className="w-12 h-12 text-slate-300 mx-auto mb-3" />
            <h3 className="text-base font-bold text-slate-800">No media assets found</h3>
            <p className="text-xs text-slate-500 mt-1 max-w-sm mx-auto">
              Upload images, PDFs, or documents to manage and embed them in your articles and pages.
            </p>
            <Button
              variant="primary"
              size="sm"
              className="mt-4"
              onClick={() => fileInputRef.current?.click()}
            >
              Upload Asset
            </Button>
          </CardContent>
        </Card>
      ) : viewMode === 'grid' ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-5">
          {filteredAssets.map((asset) => (
            <Card key={asset.id} className="group relative flex flex-col justify-between">
              <div>
                {/* Preview Thumbnail */}
                <div className="h-44 bg-slate-100 border-b border-slate-100 flex items-center justify-center overflow-hidden relative">
                  {isImage(asset.mime_type) ? (
                    <img
                      src={asset.url}
                      alt={asset.metadata?.alt_text || asset.original_name}
                      className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
                      onError={(e) => {
                        // Fallback icon if image cannot be rendered directly
                        (e.target as HTMLElement).style.display = 'none';
                      }}
                    />
                  ) : (
                    <File className="w-12 h-12 text-slate-400" />
                  )}
                  <span className="absolute top-2 right-2 px-1.5 py-0.5 rounded text-[10px] font-mono font-semibold bg-slate-900/70 text-white backdrop-blur-xs">
                    {formatFileSize(asset.file_size)}
                  </span>
                </div>

                <div className="p-4 space-y-2">
                  <h4 className="text-xs font-bold text-slate-900 truncate" title={asset.original_name}>
                    {asset.original_name}
                  </h4>
                  <div className="flex items-center justify-between text-[11px] text-slate-500">
                    <span className="font-mono">{asset.mime_type}</span>
                    {asset.metadata?.width && asset.metadata?.height && (
                      <span>
                        {asset.metadata.width}×{asset.metadata.height}
                      </span>
                    )}
                  </div>
                </div>
              </div>

              {/* Action Buttons */}
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
                    onClick={() => handleOpenSignedModal(asset)}
                    title="Generate Signed URL"
                  >
                    <KeyRound className="w-3.5 h-3.5 text-sky-600" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleDelete(asset.id, asset.original_name)}
                    title="Delete Asset"
                  >
                    <Trash2 className="w-3.5 h-3.5 text-rose-600" />
                  </Button>
                </div>
              </div>
            </Card>
          ))}
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
                  <th className="px-6 py-3">File Size</th>
                  <th className="px-6 py-3">Storage Key</th>
                  <th className="px-6 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 text-slate-700">
                {filteredAssets.map((asset) => (
                  <tr key={asset.id} className="hover:bg-slate-50/50">
                    <td className="px-6 py-3.5">
                      <div className="flex items-center gap-3">
                        <div className="w-9 h-9 rounded-lg bg-slate-100 overflow-hidden shrink-0 flex items-center justify-center">
                          {isImage(asset.mime_type) ? (
                            <img src={asset.url} alt="" className="w-full h-full object-cover" />
                          ) : (
                            <FileText className="w-4 h-4 text-slate-400" />
                          )}
                        </div>
                        <div className="truncate max-w-xs">
                          <p className="font-bold text-slate-900 truncate">{asset.original_name}</p>
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
                        onClick={() => handleOpenSignedModal(asset)}
                      >
                        <KeyRound className="w-3.5 h-3.5 text-sky-600" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(asset.id, asset.original_name)}
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
