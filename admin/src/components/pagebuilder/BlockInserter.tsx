import React, { useState } from 'react';
import {
  Search,
  Heading,
  Type,
  Quote,
  Image,
  Layout,
  Box,
  Columns3,
  Minus,
  MoveVertical,
  MousePointerClick,
  Database,
  X,
} from 'lucide-react';
import { BLOCK_DEFINITIONS } from './BlockDefinitions';
import type { BlockDefinition } from './types';

const ICON_MAP: Record<string, React.FC<{ className?: string }>> = {
  Heading,
  Type,
  Quote,
  Image,
  Layout,
  Box,
  Columns3,
  Minus,
  MoveVertical,
  MousePointerClick,
  Database,
};

export interface BlockInserterProps {
  isOpen: boolean;
  onClose: () => void;
  onInsertBlock: (def: BlockDefinition) => void;
}

export const BlockInserter: React.FC<BlockInserterProps> = ({
  isOpen,
  onClose,
  onInsertBlock,
}) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [activeCategory, setActiveCategory] = useState<string>('all');

  if (!isOpen) return null;

  const categories: { key: string; label: string }[] = [
    { key: 'all', label: 'All' },
    { key: 'text', label: 'Text' },
    { key: 'media', label: 'Media' },
    { key: 'layout', label: 'Layout' },
    { key: 'dynamic', label: 'Dynamic' },
  ];

  const filteredBlocks = BLOCK_DEFINITIONS.filter((def) => {
    const matchesSearch =
      def.label.toLowerCase().includes(searchTerm.toLowerCase()) ||
      def.description.toLowerCase().includes(searchTerm.toLowerCase());
    const matchesCategory =
      activeCategory === 'all' || def.category === activeCategory;
    return matchesSearch && matchesCategory;
  });

  return (
    <aside className="w-80 h-full bg-white border-r border-slate-200 flex flex-col shadow-lg z-20 shrink-0">
      {/* Header */}
      <div className="p-4 border-b border-slate-200 flex items-center justify-between">
        <div>
          <h2 className="text-sm font-bold text-slate-800">Block Library</h2>
          <p className="text-xs text-slate-500">Click a block to insert into canvas</p>
        </div>
        <button
          onClick={onClose}
          className="p-1 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100 cursor-pointer"
        >
          <X className="w-4 h-4" />
        </button>
      </div>

      {/* Search Input */}
      <div className="p-3 border-b border-slate-100">
        <div className="relative">
          <Search className="w-4 h-4 absolute left-3 top-2.5 text-slate-400" />
          <input
            type="text"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            placeholder="Search blocks..."
            className="w-full pl-9 pr-3 py-1.5 text-xs bg-slate-50 border border-slate-200 rounded-lg focus:outline-none focus:ring-1 focus:ring-sky-500 focus:bg-white transition-all"
          />
        </div>

        {/* Category Pills */}
        <div className="flex gap-1 mt-2.5 overflow-x-auto pb-1 text-[11px]">
          {categories.map((cat) => (
            <button
              key={cat.key}
              onClick={() => setActiveCategory(cat.key)}
              className={`px-2.5 py-1 rounded-md font-medium transition-colors cursor-pointer ${
                activeCategory === cat.key
                  ? 'bg-sky-600 text-white'
                  : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
              }`}
            >
              {cat.label}
            </button>
          ))}
        </div>
      </div>

      {/* Block List Grid */}
      <div className="flex-1 overflow-y-auto p-3 space-y-2">
        {filteredBlocks.length === 0 ? (
          <div className="text-center py-8 text-xs text-slate-400">
            No blocks match "{searchTerm}".
          </div>
        ) : (
          filteredBlocks.map((def) => {
            const IconComponent = ICON_MAP[def.icon] || Box;
            return (
              <button
                key={def.type}
                onClick={() => onInsertBlock(def)}
                className="w-full text-left p-2.5 rounded-lg border border-slate-200 hover:border-sky-400 hover:bg-sky-50/50 hover:shadow-xs transition-all flex items-start gap-3 cursor-pointer group"
              >
                <div className="w-8 h-8 rounded-lg bg-slate-100 group-hover:bg-sky-100 text-slate-600 group-hover:text-sky-600 flex items-center justify-center shrink-0 transition-colors">
                  <IconComponent className="w-4 h-4" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-xs font-semibold text-slate-800 group-hover:text-sky-900 truncate">
                    {def.label}
                  </p>
                  <p className="text-[11px] text-slate-500 line-clamp-2 mt-0.5 leading-snug">
                    {def.description}
                  </p>
                </div>
              </button>
            );
          })
        )}
      </div>
    </aside>
  );
};
