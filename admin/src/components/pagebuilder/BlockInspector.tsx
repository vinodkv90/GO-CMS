import React, { useState } from 'react';
import {
  Settings,
  AlignLeft,
  AlignCenter,
  AlignRight,
  AlignJustify,
  Image as ImageIcon,
  X,
} from 'lucide-react';
import { MediaPickerModal } from '../media/MediaPickerModal';
import type { MediaAsset } from '../../types';
import type { Block, BlockAttributes, PageDocument } from './types';

export interface BlockInspectorProps {
  isOpen: boolean;
  onClose: () => void;
  selectedBlock: Block | null;
  pageDoc: PageDocument;
  onUpdateBlockAttributes: (blockId: string, newAttrs: Partial<BlockAttributes>) => void;
  onUpdatePageDoc: (updates: Partial<PageDocument>) => void;
}

export const BlockInspector: React.FC<BlockInspectorProps> = ({
  isOpen,
  onClose,
  selectedBlock,
  pageDoc,
  onUpdateBlockAttributes,
  onUpdatePageDoc,
}) => {
  const [activeTab, setActiveTab] = useState<'block' | 'page'>('block');
  const [isMediaPickerOpen, setIsMediaPickerOpen] = useState(false);

  if (!isOpen) return null;

  const handleMediaConfirm = (assets: MediaAsset[]) => {
    if (assets.length > 0 && selectedBlock) {
      const asset = assets[0];
      const url = asset.url || `/uploads/${asset.filename}`;
      onUpdateBlockAttributes(selectedBlock.id, {
        url,
        alt: asset.metadata?.alt_text || asset.original_name || asset.filename,
        caption: asset.metadata?.caption || '',
      });
    }
    setIsMediaPickerOpen(false);
  };

  const renderBlockInspectorContent = () => {
    if (!selectedBlock) {
      return (
        <div className="p-8 text-center text-slate-400 text-xs">
          <Settings className="w-8 h-8 mx-auto mb-2 opacity-50" />
          <p>No block selected.</p>
          <p className="mt-1">Click any block on the canvas to inspect its styles and attributes.</p>
        </div>
      );
    }

    const { type, attributes } = selectedBlock;

    return (
      <div className="p-4 space-y-5 text-xs">
        {/* Block Header */}
        <div className="pb-3 border-b border-slate-100 flex items-center justify-between">
          <div>
            <span className="text-[10px] font-bold text-sky-600 uppercase tracking-wider">
              {type.replace('core/', '')}
            </span>
            <p className="font-semibold text-slate-800 text-sm capitalize">
              {type.replace('core/', '')} Settings
            </p>
          </div>
          <span className="font-mono text-[10px] text-slate-400 bg-slate-100 px-1.5 py-0.5 rounded">
            {selectedBlock.id.slice(0, 8)}
          </span>
        </div>

        {/* Heading Block Controls */}
        {type === 'core/heading' && (
          <div className="space-y-4">
            <div>
              <label className="block font-medium text-slate-700 mb-1.5">Heading Level</label>
              <div className="grid grid-cols-6 gap-1">
                {[1, 2, 3, 4, 5, 6].map((lvl) => (
                  <button
                    key={lvl}
                    type="button"
                    onClick={() =>
                      onUpdateBlockAttributes(selectedBlock.id, {
                        level: lvl as 1 | 2 | 3 | 4 | 5 | 6,
                      })
                    }
                    className={`py-1.5 rounded font-bold cursor-pointer text-xs ${
                      (attributes.level || 2) === lvl
                        ? 'bg-sky-600 text-white shadow-xs'
                        : 'bg-slate-100 text-slate-700 hover:bg-slate-200'
                    }`}
                  >
                    H{lvl}
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1.5">Alignment</label>
              <div className="flex gap-1 bg-slate-100 p-1 rounded-lg">
                {[
                  { key: 'left', icon: AlignLeft },
                  { key: 'center', icon: AlignCenter },
                  { key: 'right', icon: AlignRight },
                ].map(({ key, icon: Icon }) => (
                  <button
                    key={key}
                    type="button"
                    onClick={() =>
                      onUpdateBlockAttributes(selectedBlock.id, {
                        align: key as 'left' | 'center' | 'right',
                      })
                    }
                    className={`flex-1 py-1 flex items-center justify-center rounded cursor-pointer ${
                      (attributes.align || 'left') === key
                        ? 'bg-white shadow-xs text-sky-600 font-bold'
                        : 'text-slate-500 hover:text-slate-800'
                    }`}
                  >
                    <Icon className="w-3.5 h-3.5" />
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1">Text Color (HEX)</label>
              <div className="flex gap-2 items-center">
                <input
                  type="color"
                  value={attributes.textColor || '#0f172a'}
                  onChange={(e) =>
                    onUpdateBlockAttributes(selectedBlock.id, { textColor: e.target.value })
                  }
                  className="w-8 h-8 rounded border border-slate-200 cursor-pointer p-0"
                />
                <input
                  type="text"
                  value={attributes.textColor || ''}
                  onChange={(e) =>
                    onUpdateBlockAttributes(selectedBlock.id, { textColor: e.target.value })
                  }
                  placeholder="#0f172a"
                  className="flex-1 px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
                />
              </div>
            </div>
          </div>
        )}

        {/* Paragraph Block Controls */}
        {type === 'core/paragraph' && (
          <div className="space-y-4">
            <div>
              <label className="block font-medium text-slate-700 mb-1.5">Alignment</label>
              <div className="flex gap-1 bg-slate-100 p-1 rounded-lg">
                {[
                  { key: 'left', icon: AlignLeft },
                  { key: 'center', icon: AlignCenter },
                  { key: 'right', icon: AlignRight },
                  { key: 'justify', icon: AlignJustify },
                ].map(({ key, icon: Icon }) => (
                  <button
                    key={key}
                    type="button"
                    onClick={() =>
                      onUpdateBlockAttributes(selectedBlock.id, {
                        align: key as any,
                      })
                    }
                    className={`flex-1 py-1 flex items-center justify-center rounded cursor-pointer ${
                      (attributes.align || 'left') === key
                        ? 'bg-white shadow-xs text-sky-600 font-bold'
                        : 'text-slate-500 hover:text-slate-800'
                    }`}
                  >
                    <Icon className="w-3.5 h-3.5" />
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1">Text Color</label>
              <div className="flex gap-2 items-center">
                <input
                  type="color"
                  value={attributes.textColor || '#334155'}
                  onChange={(e) =>
                    onUpdateBlockAttributes(selectedBlock.id, { textColor: e.target.value })
                  }
                  className="w-8 h-8 rounded border border-slate-200 cursor-pointer p-0"
                />
                <input
                  type="text"
                  value={attributes.textColor || ''}
                  onChange={(e) =>
                    onUpdateBlockAttributes(selectedBlock.id, { textColor: e.target.value })
                  }
                  placeholder="#334155"
                  className="flex-1 px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
                />
              </div>
            </div>
          </div>
        )}

        {/* Image Block Controls */}
        {type === 'core/image' && (
          <div className="space-y-4">
            <div>
              <label className="block font-medium text-slate-700 mb-1">Image Source URL</label>
              <input
                type="text"
                value={attributes.url || ''}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { url: e.target.value })
                }
                placeholder="https://... or /uploads/..."
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              />
              <button
                type="button"
                onClick={() => setIsMediaPickerOpen(true)}
                className="mt-2 w-full flex items-center justify-center gap-1.5 py-1.5 bg-sky-50 hover:bg-sky-100 text-sky-700 border border-sky-200 rounded-lg font-medium cursor-pointer transition-colors"
              >
                <ImageIcon className="w-3.5 h-3.5" />
                <span>Select from Media Library</span>
              </button>
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1">Alt Text</label>
              <input
                type="text"
                value={attributes.alt || ''}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { alt: e.target.value })
                }
                placeholder="Describe image for accessibility"
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              />
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1">Caption</label>
              <input
                type="text"
                value={attributes.caption || ''}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { caption: e.target.value })
                }
                placeholder="Optional image caption"
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              />
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1.5">Alignment</label>
              <div className="flex gap-1 bg-slate-100 p-1 rounded-lg">
                {[
                  { key: 'left', icon: AlignLeft },
                  { key: 'center', icon: AlignCenter },
                  { key: 'right', icon: AlignRight },
                ].map(({ key, icon: Icon }) => (
                  <button
                    key={key}
                    type="button"
                    onClick={() =>
                      onUpdateBlockAttributes(selectedBlock.id, {
                        align: key as any,
                      })
                    }
                    className={`flex-1 py-1 flex items-center justify-center rounded cursor-pointer ${
                      (attributes.align || 'center') === key
                        ? 'bg-white shadow-xs text-sky-600 font-bold'
                        : 'text-slate-500 hover:text-slate-800'
                    }`}
                  >
                    <Icon className="w-3.5 h-3.5" />
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}

        {/* Button Block Controls */}
        {type === 'core/button' && (
          <div className="space-y-4">
            <div>
              <label className="block font-medium text-slate-700 mb-1">Button Label</label>
              <input
                type="text"
                value={attributes.text || ''}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { text: e.target.value })
                }
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              />
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1">Target URL</label>
              <input
                type="text"
                value={attributes.url || ''}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { url: e.target.value })
                }
                placeholder="https://... or /about-us"
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              />
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1">Variant Style</label>
              <select
                value={attributes.variant || 'primary'}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { variant: e.target.value as any })
                }
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              >
                <option value="primary">Primary (Blue)</option>
                <option value="secondary">Secondary (Dark)</option>
                <option value="outline">Outline</option>
                <option value="ghost">Ghost</option>
              </select>
            </div>

            <div className="flex items-center justify-between pt-1">
              <label className="font-medium text-slate-700">Open in New Tab</label>
              <input
                type="checkbox"
                checked={Boolean(attributes.newTab)}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { newTab: e.target.checked })
                }
                className="w-4 h-4 text-sky-600 rounded border-slate-300"
              />
            </div>
          </div>
        )}

        {/* Section Block Controls */}
        {type === 'core/section' && (
          <div className="space-y-4">
            <div>
              <label className="block font-medium text-slate-700 mb-1">Vertical Padding</label>
              <select
                value={attributes.padding || 'py-12'}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { padding: e.target.value })
                }
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              >
                <option value="py-4">Compact (py-4)</option>
                <option value="py-8">Small (py-8)</option>
                <option value="py-12">Medium (py-12)</option>
                <option value="py-16">Large (py-16)</option>
                <option value="py-20">Hero (py-20)</option>
              </select>
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1">Background Color</label>
              <div className="flex gap-2 items-center">
                <input
                  type="color"
                  value={attributes.backgroundColor || '#f8fafc'}
                  onChange={(e) =>
                    onUpdateBlockAttributes(selectedBlock.id, {
                      backgroundColor: e.target.value,
                    })
                  }
                  className="w-8 h-8 rounded border border-slate-200 cursor-pointer p-0"
                />
                <input
                  type="text"
                  value={attributes.backgroundColor || ''}
                  onChange={(e) =>
                    onUpdateBlockAttributes(selectedBlock.id, {
                      backgroundColor: e.target.value,
                    })
                  }
                  placeholder="#f8fafc"
                  className="flex-1 px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
                />
              </div>
            </div>
          </div>
        )}

        {/* Container Block Controls */}
        {type === 'core/container' && (
          <div className="space-y-4">
            <div>
              <label className="block font-medium text-slate-700 mb-1">Maximum Width</label>
              <select
                value={attributes.maxWidth || 'max-w-5xl'}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { maxWidth: e.target.value })
                }
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              >
                <option value="max-w-4xl">Narrow (max-w-4xl)</option>
                <option value="max-w-5xl">Standard (max-w-5xl)</option>
                <option value="max-w-6xl">Wide (max-w-6xl)</option>
                <option value="max-w-full">Full Bleed (100%)</option>
              </select>
            </div>
          </div>
        )}

        {/* Columns Grid Controls */}
        {type === 'core/columns' && (
          <div className="space-y-4">
            <div>
              <label className="block font-medium text-slate-700 mb-1">Number of Columns</label>
              <div className="grid grid-cols-4 gap-1">
                {[1, 2, 3, 4].map((n) => (
                  <button
                    key={n}
                    type="button"
                    onClick={() =>
                      onUpdateBlockAttributes(selectedBlock.id, { columns: n })
                    }
                    className={`py-1.5 rounded font-bold cursor-pointer text-xs ${
                      (attributes.columns || 2) === n
                        ? 'bg-sky-600 text-white shadow-xs'
                        : 'bg-slate-100 text-slate-700 hover:bg-slate-200'
                    }`}
                  >
                    {n}
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1">Column Gap</label>
              <select
                value={attributes.gap || 'gap-6'}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { gap: e.target.value })
                }
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              >
                <option value="gap-4">Compact (gap-4)</option>
                <option value="gap-6">Medium (gap-6)</option>
                <option value="gap-8">Spacious (gap-8)</option>
              </select>
            </div>
          </div>
        )}

        {/* Spacer Block Controls */}
        {type === 'core/spacer' && (
          <div className="space-y-4">
            <div>
              <label className="block font-medium text-slate-700 mb-1">Height</label>
              <select
                value={attributes.height || '32px'}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, { height: e.target.value })
                }
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              >
                <option value="16px">Small (16px)</option>
                <option value="32px">Medium (32px)</option>
                <option value="64px">Large (64px)</option>
                <option value="96px">Extra Large (96px)</option>
              </select>
            </div>
          </div>
        )}

        {/* Dynamic Posts Controls */}
        {type === 'core/dynamic-posts' && (
          <div className="space-y-4">
            <div>
              <label className="block font-medium text-slate-700 mb-1">Source Collection UID</label>
              <input
                type="text"
                value={attributes.collectionUid || 'blog-posts'}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, {
                    collectionUid: e.target.value,
                  })
                }
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs font-mono"
              />
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1">Item Limit</label>
              <input
                type="number"
                min={1}
                max={24}
                value={attributes.limit || 6}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, {
                    limit: parseInt(e.target.value, 10) || 6,
                  })
                }
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              />
            </div>

            <div>
              <label className="block font-medium text-slate-700 mb-1">Display Layout</label>
              <select
                value={attributes.layout || 'grid'}
                onChange={(e) =>
                  onUpdateBlockAttributes(selectedBlock.id, {
                    layout: e.target.value as 'grid' | 'list',
                  })
                }
                className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
              >
                <option value="grid">Grid (Responsive Cards)</option>
                <option value="list">List (Vertical Stack)</option>
              </select>
            </div>
          </div>
        )}
      </div>
    );
  };

  const renderPageSettingsContent = () => (
    <div className="p-4 space-y-5 text-xs">
      <div className="pb-3 border-b border-slate-100">
        <span className="text-[10px] font-bold text-sky-600 uppercase tracking-wider">Document</span>
        <p className="font-semibold text-slate-800 text-sm">Page Configuration</p>
      </div>

      <div className="space-y-4">
        <div>
          <label className="block font-medium text-slate-700 mb-1">Page Title</label>
          <input
            type="text"
            value={pageDoc.title}
            onChange={(e) => onUpdatePageDoc({ title: e.target.value })}
            className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
          />
        </div>

        <div>
          <label className="block font-medium text-slate-700 mb-1">URL Route Path</label>
          <div className="flex items-center">
            <span className="px-2.5 py-1.5 bg-slate-100 border border-r-0 border-slate-200 rounded-l-lg text-slate-500 font-mono text-xs">
              /
            </span>
            <input
              type="text"
              value={pageDoc.slug}
              onChange={(e) => onUpdatePageDoc({ slug: e.target.value.toLowerCase().replace(/[^a-z0-9-_]/g, '') })}
              className="flex-1 px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-r-lg text-xs font-mono"
            />
          </div>
        </div>

        <div>
          <label className="block font-medium text-slate-700 mb-1">Theme Layout Template</label>
          <select
            value={pageDoc.template}
            onChange={(e) =>
              onUpdatePageDoc({
                template: e.target.value as 'default' | 'full-width' | 'landing',
              })
            }
            className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
          >
            <option value="default">Default (Boxed with Header & Footer)</option>
            <option value="full-width">Full Width (Edge-to-edge canvas)</option>
            <option value="landing">Landing Page (Marketing hero layout)</option>
          </select>
        </div>

        {/* SEO Metadata Box */}
        <div className="pt-3 border-t border-slate-100 space-y-3">
          <p className="font-semibold text-slate-800 text-xs">Search Engine Optimization (SEO)</p>
          <div>
            <label className="block font-medium text-slate-600 mb-1">Meta Title</label>
            <input
              type="text"
              value={pageDoc.seo?.meta_title || ''}
              onChange={(e) =>
                onUpdatePageDoc({
                  seo: { ...pageDoc.seo, meta_title: e.target.value },
                })
              }
              placeholder={pageDoc.title || 'Page title for search engines'}
              className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
            />
          </div>

          <div>
            <label className="block font-medium text-slate-600 mb-1">Meta Description</label>
            <textarea
              value={pageDoc.seo?.meta_description || ''}
              onChange={(e) =>
                onUpdatePageDoc({
                  seo: { ...pageDoc.seo, meta_description: e.target.value },
                })
              }
              placeholder="Short search engine snippet..."
              rows={3}
              className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs resize-none"
            />
          </div>

          <div>
            <label className="block font-medium text-slate-600 mb-1">OG Share Image URL</label>
            <input
              type="text"
              value={pageDoc.seo?.og_image || ''}
              onChange={(e) =>
                onUpdatePageDoc({
                  seo: { ...pageDoc.seo, og_image: e.target.value },
                })
              }
              placeholder="https://... or /uploads/share.jpg"
              className="w-full px-3 py-1.5 bg-slate-50 border border-slate-200 rounded-lg text-xs"
            />
          </div>
        </div>
      </div>
    </div>
  );

  return (
    <aside className="w-80 h-full bg-white border-l border-slate-200 flex flex-col shadow-lg z-20 shrink-0">
      {/* Tab Switcher */}
      <div className="p-3 border-b border-slate-200 flex items-center justify-between">
        <div className="flex gap-1 bg-slate-100 p-0.5 rounded-lg flex-1 mr-2 text-xs">
          <button
            onClick={() => setActiveTab('block')}
            className={`flex-1 py-1 rounded font-medium cursor-pointer transition-colors ${
              activeTab === 'block'
                ? 'bg-white text-slate-900 shadow-xs'
                : 'text-slate-500 hover:text-slate-900'
            }`}
          >
            Block
          </button>
          <button
            onClick={() => setActiveTab('page')}
            className={`flex-1 py-1 rounded font-medium cursor-pointer transition-colors ${
              activeTab === 'page'
                ? 'bg-white text-slate-900 shadow-xs'
                : 'text-slate-500 hover:text-slate-900'
            }`}
          >
            Page
          </button>
        </div>
        <button
          onClick={onClose}
          className="p-1 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100 cursor-pointer"
        >
          <X className="w-4 h-4" />
        </button>
      </div>

      {/* Inspector Body */}
      <div className="flex-1 overflow-y-auto">
        {activeTab === 'block' ? renderBlockInspectorContent() : renderPageSettingsContent()}
      </div>

      {/* Media Picker Modal */}
      <MediaPickerModal
        isOpen={isMediaPickerOpen}
        onClose={() => setIsMediaPickerOpen(false)}
        allowedTypes={['image']}
        multiple={false}
        onConfirm={handleMediaConfirm}
      />
    </aside>
  );
};
