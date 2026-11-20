import React from 'react';
import {
  ChevronUp,
  ChevronDown,
  Copy,
  Trash2,
  Plus,
  Image as ImageIcon,
  ExternalLink,
} from 'lucide-react';
import type { Block, BlockAttributes } from './types';

export interface BlockRendererProps {
  block: Block;
  isSelected: boolean;
  onSelect: (blockId: string, e: React.MouseEvent) => void;
  onUpdateAttributes: (blockId: string, newAttrs: Partial<BlockAttributes>) => void;
  onMoveUp: (blockId: string) => void;
  onMoveDown: (blockId: string) => void;
  onDuplicate: (blockId: string) => void;
  onDelete: (blockId: string) => void;
  onAddChildBlock?: (parentId: string) => void;
  selectedBlockId: string | null;
}

export const BlockRenderer: React.FC<BlockRendererProps> = ({
  block,
  isSelected,
  onSelect,
  onUpdateAttributes,
  onMoveUp,
  onMoveDown,
  onDuplicate,
  onDelete,
  onAddChildBlock,
  selectedBlockId,
}) => {
  const { type, attributes, children } = block;

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onSelect(block.id, e);
  };

  const renderToolbar = () => (
    <div
      onClick={(e) => e.stopPropagation()}
      className="absolute -top-10 left-2 z-30 flex items-center bg-slate-900 text-white rounded-md shadow-xl border border-slate-700 py-0.5 px-1 gap-1 text-xs"
    >
      <span className="px-2 py-0.5 font-bold uppercase tracking-wider text-[10px] text-sky-400 border-r border-slate-700">
        {type.replace('core/', '')}
      </span>
      <button
        onClick={() => onMoveUp(block.id)}
        title="Move Up"
        className="p-1 hover:bg-slate-800 rounded text-slate-300 hover:text-white cursor-pointer"
      >
        <ChevronUp className="w-3.5 h-3.5" />
      </button>
      <button
        onClick={() => onMoveDown(block.id)}
        title="Move Down"
        className="p-1 hover:bg-slate-800 rounded text-slate-300 hover:text-white cursor-pointer"
      >
        <ChevronDown className="w-3.5 h-3.5" />
      </button>
      <button
        onClick={() => onDuplicate(block.id)}
        title="Duplicate"
        className="p-1 hover:bg-slate-800 rounded text-slate-300 hover:text-white cursor-pointer"
      >
        <Copy className="w-3.5 h-3.5" />
      </button>
      <button
        onClick={() => onDelete(block.id)}
        title="Delete Block"
        className="p-1 hover:bg-rose-950/60 rounded text-rose-400 hover:text-rose-300 cursor-pointer"
      >
        <Trash2 className="w-3.5 h-3.5" />
      </button>
    </div>
  );

  const wrapperClasses = `relative transition-all duration-150 rounded ${
    isSelected
      ? 'ring-2 ring-sky-500 ring-offset-2 ring-offset-white'
      : 'hover:ring-1 hover:ring-slate-300'
  }`;

  switch (type) {
    case 'core/section': {
      const padding = attributes.padding || 'py-12';
      const bgColor = attributes.backgroundColor || 'transparent';
      const textColor = attributes.textColor || 'inherit';

      return (
        <section
          onClick={handleClick}
          style={{ backgroundColor: bgColor, color: textColor }}
          className={`${wrapperClasses} ${padding} w-full my-2`}
        >
          {isSelected && renderToolbar()}
          <div className="w-full">
            {children && children.length > 0 ? (
              <div className="space-y-4">
                {children.map((child) => (
                  <BlockRenderer
                    key={child.id}
                    block={child}
                    isSelected={selectedBlockId === child.id}
                    onSelect={onSelect}
                    onUpdateAttributes={onUpdateAttributes}
                    onMoveUp={onMoveUp}
                    onMoveDown={onMoveDown}
                    onDuplicate={onDuplicate}
                    onDelete={onDelete}
                    onAddChildBlock={onAddChildBlock}
                    selectedBlockId={selectedBlockId}
                  />
                ))}
              </div>
            ) : (
              <div className="border border-dashed border-slate-300 rounded-lg p-6 text-center text-slate-400 text-sm">
                Empty Section. Click below to add blocks inside.
              </div>
            )}
            {onAddChildBlock && (
              <div className="mt-3 text-center">
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    onAddChildBlock(block.id);
                  }}
                  className="inline-flex items-center gap-1.5 px-3 py-1 text-xs font-medium text-sky-600 bg-sky-50 hover:bg-sky-100 rounded-md border border-sky-200 transition-colors cursor-pointer"
                >
                  <Plus className="w-3.5 h-3.5" />
                  <span>Add block in section</span>
                </button>
              </div>
            )}
          </div>
        </section>
      );
    }

    case 'core/container': {
      const maxWidth = attributes.maxWidth || 'max-w-5xl';
      const padding = attributes.padding || 'px-4';

      return (
        <div
          onClick={handleClick}
          className={`${wrapperClasses} ${maxWidth} mx-auto ${padding} my-2`}
        >
          {isSelected && renderToolbar()}
          {children && children.length > 0 ? (
            <div className="space-y-4">
              {children.map((child) => (
                <BlockRenderer
                  key={child.id}
                  block={child}
                  isSelected={selectedBlockId === child.id}
                  onSelect={onSelect}
                  onUpdateAttributes={onUpdateAttributes}
                  onMoveUp={onMoveUp}
                  onMoveDown={onMoveDown}
                  onDuplicate={onDuplicate}
                  onDelete={onDelete}
                  onAddChildBlock={onAddChildBlock}
                  selectedBlockId={selectedBlockId}
                />
              ))}
            </div>
          ) : (
            <div className="border border-dashed border-slate-300 rounded-lg p-4 text-center text-slate-400 text-xs">
              Empty Container
            </div>
          )}
          {onAddChildBlock && (
            <div className="mt-2 text-center">
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onAddChildBlock(block.id);
                }}
                className="inline-flex items-center gap-1 px-2.5 py-1 text-[11px] font-medium text-slate-600 bg-slate-100 hover:bg-slate-200 rounded border border-slate-300 transition-colors cursor-pointer"
              >
                <Plus className="w-3 h-3" />
                <span>Add in container</span>
              </button>
            </div>
          )}
        </div>
      );
    }

    case 'core/columns': {
      const cols = attributes.columns || 2;
      const gap = attributes.gap || 'gap-6';
      const gridClass =
        cols === 1
          ? 'grid-cols-1'
          : cols === 3
            ? 'grid-cols-1 md:grid-cols-3'
            : cols === 4
              ? 'grid-cols-1 md:grid-cols-4'
              : 'grid-cols-1 md:grid-cols-2';

      return (
        <div onClick={handleClick} className={`${wrapperClasses} grid ${gridClass} ${gap} my-4`}>
          {isSelected && renderToolbar()}
          {children && children.length > 0 ? (
            children.map((child) => (
              <BlockRenderer
                key={child.id}
                block={child}
                isSelected={selectedBlockId === child.id}
                onSelect={onSelect}
                onUpdateAttributes={onUpdateAttributes}
                onMoveUp={onMoveUp}
                onMoveDown={onMoveDown}
                onDuplicate={onDuplicate}
                onDelete={onDelete}
                onAddChildBlock={onAddChildBlock}
                selectedBlockId={selectedBlockId}
              />
            ))
          ) : (
            <div className="col-span-full border border-dashed border-slate-300 rounded p-4 text-center text-xs text-slate-400">
              Columns Grid
            </div>
          )}
        </div>
      );
    }

    case 'core/column': {
      return (
        <div
          onClick={handleClick}
          className={`${wrapperClasses} border border-dashed border-slate-200 rounded-lg p-3 min-h-[80px] bg-white/50`}
        >
          {isSelected && renderToolbar()}
          {children && children.length > 0 ? (
            <div className="space-y-3">
              {children.map((child) => (
                <BlockRenderer
                  key={child.id}
                  block={child}
                  isSelected={selectedBlockId === child.id}
                  onSelect={onSelect}
                  onUpdateAttributes={onUpdateAttributes}
                  onMoveUp={onMoveUp}
                  onMoveDown={onMoveDown}
                  onDuplicate={onDuplicate}
                  onDelete={onDelete}
                  onAddChildBlock={onAddChildBlock}
                  selectedBlockId={selectedBlockId}
                />
              ))}
            </div>
          ) : (
            <div className="text-center text-slate-400 text-xs py-4">Empty Column</div>
          )}
          {onAddChildBlock && (
            <div className="mt-2 text-center">
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onAddChildBlock(block.id);
                }}
                className="inline-flex items-center gap-1 px-2 py-0.5 text-[10px] text-slate-500 hover:text-slate-800 bg-slate-100 hover:bg-slate-200 rounded cursor-pointer"
              >
                <Plus className="w-3 h-3" />
                <span>Add</span>
              </button>
            </div>
          )}
        </div>
      );
    }

    case 'core/heading': {
      const level = attributes.level || 2;
      const align = attributes.align || 'left';
      const textColor = attributes.textColor || 'inherit';
      const alignClass = `text-${align}`;

      const headingClasses = `font-bold tracking-tight ${alignClass} ${
        level === 1
          ? 'text-3xl md:text-4xl'
          : level === 2
            ? 'text-2xl md:text-3xl'
            : level === 3
              ? 'text-xl md:text-2xl'
              : level === 4
                ? 'text-lg md:text-xl'
                : 'text-base md:text-lg'
      }`;

      return (
        <div onClick={handleClick} className={`${wrapperClasses} p-1 my-1`}>
          {isSelected && renderToolbar()}
          <input
            type="text"
            value={attributes.content || ''}
            onChange={(e) => onUpdateAttributes(block.id, { content: e.target.value })}
            style={{ color: textColor }}
            placeholder={`Heading Level ${level}`}
            className={`${headingClasses} w-full bg-transparent border-0 outline-none focus:ring-1 focus:ring-sky-400 rounded px-1`}
          />
        </div>
      );
    }

    case 'core/paragraph': {
      const align = attributes.align || 'left';
      const textColor = attributes.textColor || 'inherit';
      const alignClass = `text-${align}`;

      return (
        <div onClick={handleClick} className={`${wrapperClasses} p-1 my-1`}>
          {isSelected && renderToolbar()}
          <textarea
            value={attributes.content || ''}
            onChange={(e) => onUpdateAttributes(block.id, { content: e.target.value })}
            style={{ color: textColor }}
            placeholder="Type your paragraph here..."
            rows={Math.max(2, (attributes.content || '').split('\n').length)}
            className={`w-full bg-transparent border-0 outline-none focus:ring-1 focus:ring-sky-400 rounded p-1 text-slate-700 leading-relaxed text-sm md:text-base resize-y ${alignClass}`}
          />
        </div>
      );
    }

    case 'core/image': {
      const url = attributes.url || '';
      const alt = attributes.alt || '';
      const caption = attributes.caption || '';
      const align = attributes.align || 'center';
      const alignClass = `text-${align}`;

      return (
        <figure onClick={handleClick} className={`${wrapperClasses} ${alignClass} p-2 my-3`}>
          {isSelected && renderToolbar()}
          {url ? (
            <img
              src={url}
              alt={alt}
              className="inline-block max-w-full h-auto rounded-lg shadow-sm border border-slate-200"
            />
          ) : (
            <div className="border-2 border-dashed border-slate-300 rounded-lg p-8 flex flex-col items-center justify-center text-slate-400 bg-slate-50">
              <ImageIcon className="w-8 h-8 mb-2 text-slate-400" />
              <p className="text-xs font-medium">No image URL configured.</p>
              <p className="text-[11px] text-slate-400">Select an asset from the block inspector.</p>
            </div>
          )}
          {caption && (
            <figcaption className="text-xs text-slate-500 italic mt-2">{caption}</figcaption>
          )}
        </figure>
      );
    }

    case 'core/button': {
      const text = attributes.text || 'Button';
      const variant = attributes.variant || 'primary';
      const align = attributes.align || 'left';
      const alignClass = `text-${align}`;

      const btnStyles =
        variant === 'primary'
          ? 'bg-sky-600 text-white hover:bg-sky-700 shadow-sm'
          : variant === 'secondary'
            ? 'bg-slate-800 text-white hover:bg-slate-900 shadow-sm'
            : variant === 'outline'
              ? 'border-2 border-sky-600 text-sky-600 hover:bg-sky-50'
              : 'text-slate-700 hover:bg-slate-100';

      return (
        <div onClick={handleClick} className={`${wrapperClasses} ${alignClass} p-1 my-2`}>
          {isSelected && renderToolbar()}
          <span
            className={`inline-flex items-center gap-1.5 px-5 py-2.5 rounded-lg font-semibold text-sm transition-all pointer-events-none select-none ${btnStyles}`}
          >
            <span>{text}</span>
            {attributes.url && attributes.url !== '#' && (
              <ExternalLink className="w-3.5 h-3.5 opacity-70" />
            )}
          </span>
        </div>
      );
    }

    case 'core/separator': {
      const style = attributes.style || 'solid';
      const borderClass =
        style === 'dashed'
          ? 'border-dashed'
          : style === 'dots'
            ? 'border-dotted'
            : 'border-solid';

      return (
        <div onClick={handleClick} className={`${wrapperClasses} py-3 my-2`}>
          {isSelected && renderToolbar()}
          <hr className={`border-t border-slate-300 ${borderClass}`} />
        </div>
      );
    }

    case 'core/spacer': {
      const height = attributes.height || '32px';

      return (
        <div
          onClick={handleClick}
          style={{ height }}
          className={`${wrapperClasses} w-full flex items-center justify-center bg-slate-50/50 border border-dashed border-slate-200 rounded my-1`}
        >
          {isSelected && renderToolbar()}
          <span className="text-[10px] text-slate-400 font-mono select-none">
            Spacer ({height})
          </span>
        </div>
      );
    }

    case 'core/quote': {
      const content = attributes.content || '';
      const citation = attributes.citation || '';

      return (
        <blockquote
          onClick={handleClick}
          className={`${wrapperClasses} border-l-4 border-sky-500 bg-slate-50 p-4 my-3 rounded-r-lg`}
        >
          {isSelected && renderToolbar()}
          <textarea
            value={content}
            onChange={(e) => onUpdateAttributes(block.id, { content: e.target.value })}
            placeholder="Quote text..."
            rows={2}
            className="w-full bg-transparent border-0 outline-none text-slate-800 italic text-base leading-relaxed resize-none focus:ring-1 focus:ring-sky-400 rounded p-1"
          />
          <input
            type="text"
            value={citation}
            onChange={(e) => onUpdateAttributes(block.id, { citation: e.target.value })}
            placeholder="Citation author..."
            className="w-full bg-transparent border-0 outline-none text-xs text-slate-500 font-medium not-italic mt-2 focus:ring-1 focus:ring-sky-400 rounded px-1"
          />
        </blockquote>
      );
    }

    case 'core/dynamic-posts': {
      const collectionUid = attributes.collectionUid || 'blog-posts';
      const limit = attributes.limit || 6;
      const layout = attributes.layout || 'grid';

      return (
        <div
          onClick={handleClick}
          className={`${wrapperClasses} border border-sky-200 bg-sky-50/40 rounded-xl p-5 my-4`}
        >
          {isSelected && renderToolbar()}
          <div className="flex items-center justify-between pb-3 border-b border-sky-200 mb-3">
            <div className="flex items-center gap-2">
              <span className="w-2.5 h-2.5 rounded-full bg-sky-500 animate-pulse" />
              <span className="text-xs font-bold text-sky-900 uppercase tracking-wider">
                Dynamic CMS Posts Feed
              </span>
            </div>
            <span className="text-[11px] text-sky-700 font-mono">
              type: {collectionUid} &bull; limit: {limit} &bull; {layout}
            </span>
          </div>

          <div
            className={
              layout === 'grid'
                ? 'grid grid-cols-1 md:grid-cols-3 gap-3'
                : 'space-y-2'
            }
          >
            {[1, 2, 3].slice(0, Math.min(3, limit)).map((i) => (
              <div key={i} className="bg-white p-3 rounded-lg border border-slate-200 shadow-xs">
                <div className="h-4 bg-slate-200 rounded w-3/4 mb-2 animate-pulse" />
                <div className="h-3 bg-slate-100 rounded w-full mb-1 animate-pulse" />
                <div className="h-3 bg-slate-100 rounded w-1/2 animate-pulse" />
              </div>
            ))}
          </div>
          <p className="text-[11px] text-sky-600 mt-3 text-center italic">
            Live articles from collection "{collectionUid}" will render here dynamically on the public page.
          </p>
        </div>
      );
    }

    default:
      return (
        <div
          onClick={handleClick}
          className={`${wrapperClasses} p-3 border border-dashed border-amber-300 bg-amber-50 rounded text-xs text-amber-800`}
        >
          {isSelected && renderToolbar()}
          Unknown Block: {type}
        </div>
      );
  }
};

