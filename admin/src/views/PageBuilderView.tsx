import React, { useState, useEffect, useCallback } from 'react';
import {
  ArrowLeft,
  Monitor,
  Tablet,
  Smartphone,
  Plus,
  Sliders,
  Eye,
  Save,
  Send,
  Undo2,
  Redo2,
  Globe,
} from 'lucide-react';
import { entriesApi } from '../api/endpoints';
import { useToast } from '../components/ui/Toast';
import { BlockRenderer } from '../components/pagebuilder/BlockRenderer';
import { BlockInserter } from '../components/pagebuilder/BlockInserter';
import { BlockInspector } from '../components/pagebuilder/BlockInspector';
import type {
  Block,
  BlockAttributes,
  BlockDefinition,
  DeviceMode,
  PageDocument,
} from '../components/pagebuilder/types';

export interface PageBuilderViewProps {
  pageId?: string;
  initialSlug?: string;
  onBack: () => void;
}

export const PageBuilderView: React.FC<PageBuilderViewProps> = ({
  pageId,
  initialSlug,
  onBack,
}) => {
  const toast = useToast();

  const [pageDoc, setPageDoc] = useState<PageDocument>({
    title: 'Untitled Page',
    slug: initialSlug || 'new-page',
    template: 'default',
    status: 'draft',
    blocks: [],
    seo: {},
  });

  const [selectedBlockId, setSelectedBlockId] = useState<string | null>(null);
  const [deviceMode, setDeviceMode] = useState<DeviceMode>('desktop');
  const [isInserterOpen, setIsInserterOpen] = useState(true);
  const [isInspectorOpen, setIsInspectorOpen] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [isPublishing, setIsPublishing] = useState(false);
  const [isLoading, setIsLoading] = useState(Boolean(pageId));

  // Undo / Redo history stack
  const [history, setHistory] = useState<Block[][]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);

  // Load existing page if pageId is passed
  useEffect(() => {
    if (!pageId) {
      // Default template blocks for new pages
      const defaultBlocks: Block[] = [
        {
          id: `sec-${Date.now()}`,
          type: 'core/section',
          attributes: {
            padding: 'py-16',
            backgroundColor: '#f8fafc',
          },
          children: [
            {
              id: `cont-${Date.now() + 1}`,
              type: 'core/container',
              attributes: { maxWidth: 'max-w-5xl' },
              children: [
                {
                  id: `h-${Date.now() + 2}`,
                  type: 'core/heading',
                  attributes: {
                    level: 1,
                    content: 'Welcome to Your New Page',
                    align: 'center',
                  },
                },
                {
                  id: `p-${Date.now() + 3}`,
                  type: 'core/paragraph',
                  attributes: {
                    content:
                      'Visually customize this page using Gutenberg-style blocks. Click any element to edit in place or tweak styles in the inspector.',
                    align: 'center',
                  },
                },
                {
                  id: `btn-${Date.now() + 4}`,
                  type: 'core/button',
                  attributes: {
                    text: 'Explore Features',
                    url: '#',
                    variant: 'primary',
                    align: 'center',
                  },
                },
              ],
            },
          ],
        },
      ];
      setPageDoc((prev) => ({ ...prev, blocks: defaultBlocks }));
      setHistory([defaultBlocks]);
      setHistoryIndex(0);
      return;
    }

    const loadPage = async () => {
      try {
        setIsLoading(true);
        const entry = await entriesApi.get('pages', pageId);
        const data = entry.published_data || entry.draft_data || {};

        setPageDoc({
          id: entry.id,
          title: (data.title as string) || 'Untitled Page',
          slug: entry.slug || (data.slug as string) || 'page',
          template: (data.template as any) || 'default',
          status: entry.status === 'published' ? 'published' : 'draft',
          blocks: Array.isArray(data.blocks) ? (data.blocks as Block[]) : [],
          seo: (data.seo as any) || {},
          draft_version: entry.draft_version,
          published_version: entry.published_version,
        });

        if (Array.isArray(data.blocks)) {
          setHistory([data.blocks as Block[]]);
          setHistoryIndex(0);
        }
      } catch (err: any) {
        toast.error('Failed to load page', err.message || 'Could not fetch page details from server.');
      } finally {
        setIsLoading(false);
      }
    };

    loadPage();
  }, [pageId, toast]);

  // Update block state and push to history
  const updateBlocksWithHistory = useCallback(
    (newBlocks: Block[]) => {
      setPageDoc((prev) => ({ ...prev, blocks: newBlocks }));
      setHistory((prev) => {
        const sliced = prev.slice(0, historyIndex + 1);
        return [...sliced, newBlocks];
      });
      setHistoryIndex((prev) => prev + 1);
    },
    [historyIndex]
  );

  const handleUndo = () => {
    if (historyIndex > 0) {
      const nextIndex = historyIndex - 1;
      setHistoryIndex(nextIndex);
      setPageDoc((prev) => ({ ...prev, blocks: history[nextIndex] }));
    }
  };

  const handleRedo = () => {
    if (historyIndex < history.length - 1) {
      const nextIndex = historyIndex + 1;
      setHistoryIndex(nextIndex);
      setPageDoc((prev) => ({ ...prev, blocks: history[nextIndex] }));
    }
  };

  // Helper: recursive update of block attributes
  const updateBlockInTree = (
    blocks: Block[],
    id: string,
    attrs: Partial<BlockAttributes>
  ): Block[] => {
    return blocks.map((b) => {
      if (b.id === id) {
        return { ...b, attributes: { ...b.attributes, ...attrs } };
      }
      if (b.children && b.children.length > 0) {
        return { ...b, children: updateBlockInTree(b.children, id, attrs) };
      }
      return b;
    });
  };

  // Helper: recursive deletion
  const deleteBlockFromTree = (blocks: Block[], id: string): Block[] => {
    return blocks
      .filter((b) => b.id !== id)
      .map((b) => {
        if (b.children && b.children.length > 0) {
          return { ...b, children: deleteBlockFromTree(b.children, id) };
        }
        return b;
      });
  };

  // Helper: recursive duplicate
  const duplicateBlockInTree = (blocks: Block[], id: string): Block[] => {
    const result: Block[] = [];
    for (const b of blocks) {
      result.push(b);
      if (b.id === id) {
        const clone: Block = JSON.parse(JSON.stringify(b));
        clone.id = `blk-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`;
        result.push(clone);
      } else if (b.children && b.children.length > 0) {
        b.children = duplicateBlockInTree(b.children, id);
      }
    }
    return result;
  };

  // Helper: move up / down
  const moveBlockInTree = (
    blocks: Block[],
    id: string,
    direction: 'up' | 'down'
  ): Block[] => {
    const idx = blocks.findIndex((b) => b.id === id);
    if (idx !== -1) {
      const newBlocks = [...blocks];
      const targetIdx = direction === 'up' ? idx - 1 : idx + 1;
      if (targetIdx >= 0 && targetIdx < newBlocks.length) {
        const temp = newBlocks[idx];
        newBlocks[idx] = newBlocks[targetIdx];
        newBlocks[targetIdx] = temp;
        return newBlocks;
      }
      return blocks;
    }
    return blocks.map((b) => {
      if (b.children && b.children.length > 0) {
        return { ...b, children: moveBlockInTree(b.children, id, direction) };
      }
      return b;
    });
  };

  // Helper: add child to container
  const addChildToTree = (blocks: Block[], parentId: string, newBlock: Block): Block[] => {
    return blocks.map((b) => {
      if (b.id === parentId) {
        return {
          ...b,
          children: [...(b.children || []), newBlock],
        };
      }
      if (b.children && b.children.length > 0) {
        return { ...b, children: addChildToTree(b.children, parentId, newBlock) };
      }
      return b;
    });
  };

  // Handler: Insert block from library
  const handleInsertBlock = (def: BlockDefinition) => {
    const newBlock: Block = {
      id: `blk-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
      type: def.type,
      attributes: { ...def.defaultAttributes },
      children: def.defaultChildren ? JSON.parse(JSON.stringify(def.defaultChildren)) : undefined,
    };

    if (selectedBlockId) {
      // If a container or section is selected, insert as child; otherwise insert after it
      const updated = pageDoc.blocks.map((b) => {
        if (
          b.id === selectedBlockId &&
          (b.type === 'core/section' || b.type === 'core/container' || b.type === 'core/column')
        ) {
          return { ...b, children: [...(b.children || []), newBlock] };
        }
        return b;
      });
      // If not added as child, append to root
      if (JSON.stringify(updated) === JSON.stringify(pageDoc.blocks)) {
        updateBlocksWithHistory([...pageDoc.blocks, newBlock]);
      } else {
        updateBlocksWithHistory(updated);
      }
    } else {
      updateBlocksWithHistory([...pageDoc.blocks, newBlock]);
    }

    setSelectedBlockId(newBlock.id);
    toast.success('Block Added', `Inserted ${def.label} block into the canvas.`);
  };

  // Save Page Draft to backend
  const handleSaveDraft = async () => {
    try {
      setIsSaving(true);
      const payload = {
        title: pageDoc.title,
        slug: pageDoc.slug,
        template: pageDoc.template,
        blocks: pageDoc.blocks,
        seo: pageDoc.seo,
      };

      if (pageDoc.id) {
        await entriesApi.update('pages', pageDoc.id, payload, pageDoc.draft_version || 1);
      } else {
        const created = await entriesApi.create('pages', payload);
        setPageDoc((prev) => ({ ...prev, id: created.id }));
      }

      toast.success('Draft Saved', `Page "${pageDoc.title}" draft was saved successfully.`);
    } catch (err: any) {
      toast.error('Failed to Save Draft', err.message || 'Could not save page to backend.');
    } finally {
      setIsSaving(false);
    }
  };

  // Publish Page
  const handlePublish = async () => {
    try {
      setIsPublishing(true);
      // 1. Ensure latest draft is saved first
      let currentId = pageDoc.id;
      const payload = {
        title: pageDoc.title,
        slug: pageDoc.slug,
        template: pageDoc.template,
        blocks: pageDoc.blocks,
        seo: pageDoc.seo,
      };

      if (currentId) {
        await entriesApi.update('pages', currentId, payload, pageDoc.draft_version || 1);
      } else {
        const created = await entriesApi.create('pages', payload);
        currentId = created.id;
        setPageDoc((prev) => ({ ...prev, id: currentId }));
      }

      // 2. Publish entry
      await entriesApi.publish('pages', currentId!);
      setPageDoc((prev) => ({ ...prev, status: 'published' }));

      toast.success('Page Published!', `Page is now live and routable at /${pageDoc.slug}.`);
    } catch (err: any) {
      toast.error('Failed to Publish Page', err.message || 'Could not publish page.');
    } finally {
      setIsPublishing(false);
    }
  };

  // Unpublish Page
  const handleUnpublish = async () => {
    if (!pageDoc.id) return;
    try {
      setIsPublishing(true);
      await entriesApi.unpublish('pages', pageDoc.id);
      setPageDoc((prev) => ({ ...prev, status: 'draft' }));
      toast.info('Page Unpublished', 'Page has been reverted to draft.');
    } catch (err: any) {
      toast.error('Failed to Unpublish', err.message || 'Could not unpublish page.');
    } finally {
      setIsPublishing(false);
    }
  };

  // Find currently selected block
  const findBlock = (blocks: Block[], id: string | null): Block | null => {
    if (!id) return null;
    for (const b of blocks) {
      if (b.id === id) return b;
      if (b.children) {
        const found = findBlock(b.children, id);
        if (found) return found;
      }
    }
    return null;
  };
  const activeSelectedBlock = findBlock(pageDoc.blocks, selectedBlockId);

  // Viewport width styling
  const canvasWidthClass =
    deviceMode === 'mobile'
      ? 'max-w-[375px]'
      : deviceMode === 'tablet'
        ? 'max-w-[768px]'
        : 'max-w-5xl';

  if (isLoading) {
    return (
      <div className="h-full flex items-center justify-center bg-slate-100">
        <div className="flex flex-col items-center gap-2">
          <div className="w-8 h-8 rounded-full border-2 border-sky-600 border-t-transparent animate-spin" />
          <p className="text-xs text-slate-500 font-mono">Loading Page Builder...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="h-screen w-full flex flex-col bg-slate-100 overflow-hidden select-none">
      {/* Top Application Bar */}
      <header className="h-14 bg-white border-b border-slate-200 px-4 flex items-center justify-between shadow-xs z-30 shrink-0">
        {/* Left: Back & Title */}
        <div className="flex items-center gap-3 min-w-0">
          <button
            onClick={onBack}
            className="p-1.5 rounded-lg text-slate-500 hover:text-slate-800 hover:bg-slate-100 transition-colors cursor-pointer flex items-center gap-1 text-xs font-medium"
          >
            <ArrowLeft className="w-4 h-4" />
            <span className="hidden sm:inline">Pages</span>
          </button>
          <div className="h-5 w-px bg-slate-200 hidden sm:block" />
          <div className="flex items-center gap-2 min-w-0">
            <input
              type="text"
              value={pageDoc.title}
              onChange={(e) => setPageDoc((prev) => ({ ...prev, title: e.target.value }))}
              placeholder="Page Title"
              className="font-bold text-sm text-slate-900 bg-transparent border-0 outline-none hover:bg-slate-50 focus:bg-white focus:ring-1 focus:ring-sky-500 rounded px-2 py-1 truncate"
            />
            <span
              className={`text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full ${
                pageDoc.status === 'published'
                  ? 'bg-emerald-50 text-emerald-700 border border-emerald-200'
                  : 'bg-amber-50 text-amber-700 border border-amber-200'
              }`}
            >
              {pageDoc.status}
            </span>
          </div>
        </div>

        {/* Center: Device Viewport Switcher & Undo/Redo */}
        <div className="flex items-center gap-2">
          <div className="flex items-center bg-slate-100 p-0.5 rounded-lg border border-slate-200">
            <button
              onClick={() => setDeviceMode('desktop')}
              title="Desktop View (100%)"
              className={`p-1.5 rounded-md cursor-pointer transition-colors ${
                deviceMode === 'desktop'
                  ? 'bg-white shadow-xs text-sky-600 font-bold'
                  : 'text-slate-500 hover:text-slate-800'
              }`}
            >
              <Monitor className="w-4 h-4" />
            </button>
            <button
              onClick={() => setDeviceMode('tablet')}
              title="Tablet View (768px)"
              className={`p-1.5 rounded-md cursor-pointer transition-colors ${
                deviceMode === 'tablet'
                  ? 'bg-white shadow-xs text-sky-600 font-bold'
                  : 'text-slate-500 hover:text-slate-800'
              }`}
            >
              <Tablet className="w-4 h-4" />
            </button>
            <button
              onClick={() => setDeviceMode('mobile')}
              title="Mobile View (375px)"
              className={`p-1.5 rounded-md cursor-pointer transition-colors ${
                deviceMode === 'mobile'
                  ? 'bg-white shadow-xs text-sky-600 font-bold'
                  : 'text-slate-500 hover:text-slate-800'
              }`}
            >
              <Smartphone className="w-4 h-4" />
            </button>
          </div>

          <div className="h-5 w-px bg-slate-200 hidden md:block" />

          {/* Undo / Redo */}
          <div className="hidden md:flex items-center gap-1">
            <button
              onClick={handleUndo}
              disabled={historyIndex <= 0}
              title="Undo"
              className="p-1.5 rounded text-slate-500 hover:text-slate-800 hover:bg-slate-100 disabled:opacity-30 disabled:pointer-events-none cursor-pointer"
            >
              <Undo2 className="w-4 h-4" />
            </button>
            <button
              onClick={handleRedo}
              disabled={historyIndex >= history.length - 1}
              title="Redo"
              className="p-1.5 rounded text-slate-500 hover:text-slate-800 hover:bg-slate-100 disabled:opacity-30 disabled:pointer-events-none cursor-pointer"
            >
              <Redo2 className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Right: Actions */}
        <div className="flex items-center gap-2">
          {/* Toggle Inserter */}
          <button
            onClick={() => setIsInserterOpen(!isInserterOpen)}
            title="Toggle Block Inserter"
            className={`p-2 rounded-lg border cursor-pointer transition-colors text-xs font-medium flex items-center gap-1.5 ${
              isInserterOpen
                ? 'bg-sky-50 text-sky-700 border-sky-300'
                : 'bg-white text-slate-700 border-slate-300 hover:bg-slate-50'
            }`}
          >
            <Plus className="w-4 h-4" />
            <span className="hidden lg:inline">Blocks</span>
          </button>

          {/* Toggle Inspector */}
          <button
            onClick={() => setIsInspectorOpen(!isInspectorOpen)}
            title="Toggle Inspector"
            className={`p-2 rounded-lg border cursor-pointer transition-colors text-xs font-medium flex items-center gap-1.5 ${
              isInspectorOpen
                ? 'bg-sky-50 text-sky-700 border-sky-300'
                : 'bg-white text-slate-700 border-slate-300 hover:bg-slate-50'
            }`}
          >
            <Sliders className="w-4 h-4" />
            <span className="hidden lg:inline">Inspector</span>
          </button>

          {/* Live Preview Button */}
          <a
            href={`/${pageDoc.slug}?preview=true`}
            target="_blank"
            rel="noopener noreferrer"
            title="Live Preview"
            className="p-2 rounded-lg bg-white border border-slate-300 hover:bg-slate-50 text-slate-700 text-xs font-medium flex items-center gap-1.5 cursor-pointer transition-colors"
          >
            <Eye className="w-4 h-4 text-slate-500" />
            <span className="hidden sm:inline">Preview</span>
          </a>

          {/* Save Draft */}
          <button
            onClick={handleSaveDraft}
            disabled={isSaving}
            className="px-3 py-2 rounded-lg bg-white border border-slate-300 hover:bg-slate-50 text-slate-700 text-xs font-semibold flex items-center gap-1.5 cursor-pointer transition-colors disabled:opacity-50"
          >
            <Save className="w-3.5 h-3.5 text-slate-500" />
            <span>{isSaving ? 'Saving...' : 'Save Draft'}</span>
          </button>

          {/* Publish / Unpublish */}
          {pageDoc.status === 'published' ? (
            <button
              onClick={handleUnpublish}
              disabled={isPublishing}
              className="px-3 py-2 rounded-lg bg-amber-500 hover:bg-amber-600 text-white text-xs font-semibold flex items-center gap-1.5 cursor-pointer shadow-xs transition-colors disabled:opacity-50"
            >
              <span>Unpublish</span>
            </button>
          ) : (
            <button
              onClick={handlePublish}
              disabled={isPublishing}
              className="px-4 py-2 rounded-lg bg-sky-600 hover:bg-sky-700 text-white text-xs font-semibold flex items-center gap-1.5 cursor-pointer shadow-sm transition-colors disabled:opacity-50"
            >
              <Send className="w-3.5 h-3.5" />
              <span>{isPublishing ? 'Publishing...' : 'Publish'}</span>
            </button>
          )}
        </div>
      </header>

      {/* Main Workspace Body */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left: Inserter Library */}
        <BlockInserter
          isOpen={isInserterOpen}
          onClose={() => setIsInserterOpen(false)}
          onInsertBlock={handleInsertBlock}
        />

        {/* Center: Live Interactive Canvas */}
        <main
          onClick={() => setSelectedBlockId(null)}
          className="flex-1 overflow-y-auto p-4 sm:p-8 flex flex-col items-center justify-start bg-slate-100"
        >
          {/* Viewport Frame */}
          <div
            className={`w-full ${canvasWidthClass} bg-white min-h-[85vh] rounded-xl shadow-md border border-slate-200 transition-all duration-300 flex flex-col`}
          >
            {/* Canvas Header Banner showing route */}
            <div className="px-4 py-2 bg-slate-50 border-b border-slate-100 flex items-center justify-between text-xs text-slate-500">
              <div className="flex items-center gap-1.5 font-mono text-[11px]">
                <Globe className="w-3.5 h-3.5 text-slate-400" />
                <span>/{pageDoc.slug}</span>
              </div>
              <span className="text-[10px] text-slate-400 uppercase tracking-wider font-semibold">
                Template: {pageDoc.template}
              </span>
            </div>

            {/* Block Tree Canvas */}
            <div className="p-6 md:p-8 flex-1 space-y-4">
              {pageDoc.blocks.length === 0 ? (
                <div className="py-20 text-center flex flex-col items-center justify-center">
                  <div className="w-12 h-12 rounded-2xl bg-sky-50 text-sky-600 flex items-center justify-center mb-3">
                    <Plus className="w-6 h-6" />
                  </div>
                  <h3 className="font-bold text-slate-800 text-base mb-1">Canvas is empty</h3>
                  <p className="text-xs text-slate-500 max-w-sm mb-4">
                    Open the block library on the left or click the button below to insert your first element.
                  </p>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setIsInserterOpen(true);
                    }}
                    className="px-4 py-2 bg-sky-600 hover:bg-sky-700 text-white font-medium text-xs rounded-lg shadow-sm cursor-pointer transition-colors"
                  >
                    Open Block Library
                  </button>
                </div>
              ) : (
                pageDoc.blocks.map((block) => (
                  <BlockRenderer
                    key={block.id}
                    block={block}
                    isSelected={selectedBlockId === block.id}
                    onSelect={(id) => setSelectedBlockId(id)}
                    onUpdateAttributes={(id, attrs) =>
                      updateBlocksWithHistory(updateBlockInTree(pageDoc.blocks, id, attrs))
                    }
                    onMoveUp={(id) =>
                      updateBlocksWithHistory(moveBlockInTree(pageDoc.blocks, id, 'up'))
                    }
                    onMoveDown={(id) =>
                      updateBlocksWithHistory(moveBlockInTree(pageDoc.blocks, id, 'down'))
                    }
                    onDuplicate={(id) =>
                      updateBlocksWithHistory(duplicateBlockInTree(pageDoc.blocks, id))
                    }
                    onDelete={(id) => {
                      updateBlocksWithHistory(deleteBlockFromTree(pageDoc.blocks, id));
                      if (selectedBlockId === id) setSelectedBlockId(null);
                    }}
                    onAddChildBlock={(parentId) => {
                      setSelectedBlockId(parentId);
                      setIsInserterOpen(true);
                    }}
                    selectedBlockId={selectedBlockId}
                  />
                ))
              )}
            </div>

            {/* Bottom Add Block Quick Bar */}
            <div className="p-4 border-t border-slate-100 text-center">
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  setIsInserterOpen(true);
                }}
                className="inline-flex items-center gap-1.5 px-4 py-1.5 text-xs font-semibold text-slate-600 bg-slate-50 hover:bg-slate-100 rounded-lg border border-slate-200 transition-colors cursor-pointer"
              >
                <Plus className="w-3.5 h-3.5 text-sky-600" />
                <span>Add Block to Bottom</span>
              </button>
            </div>
          </div>
        </main>

        {/* Right: Inspector */}
        <BlockInspector
          isOpen={isInspectorOpen}
          onClose={() => setIsInspectorOpen(false)}
          selectedBlock={activeSelectedBlock}
          pageDoc={pageDoc}
          onUpdateBlockAttributes={(id, attrs) =>
            updateBlocksWithHistory(updateBlockInTree(pageDoc.blocks, id, attrs))
          }
          onUpdatePageDoc={(updates) => setPageDoc((prev) => ({ ...prev, ...updates }))}
        />
      </div>
    </div>
  );
};
