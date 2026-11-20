import React, { useState, useEffect } from 'react';
import {
  FileText,
  Plus,
  Search,
  ExternalLink,
  Edit3,
  Trash2,
  Globe,
} from 'lucide-react';
import { entriesApi } from '../api/endpoints';
import { useToast } from '../components/ui/Toast';
import { Modal } from '../components/ui/Modal';
import { Button } from '../components/ui/Button';
import type { ContentEntry } from '../types';

export interface PagesViewProps {
  onOpenBuilder: (pageId?: string, initialSlug?: string) => void;
  onNavigateToContent?: (uid: string) => void;
}

export const PagesView: React.FC<PagesViewProps> = ({ onOpenBuilder, onNavigateToContent }) => {
  const toast = useToast();
  const [pages, setPages] = useState<ContentEntry[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'published' | 'draft'>('all');

  // Create Modal state
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [newTitle, setNewTitle] = useState('');
  const [newSlug, setNewSlug] = useState('');
  const [newTemplate, setNewTemplate] = useState<'default' | 'full-width' | 'landing'>('default');
  const [isCreating, setIsCreating] = useState(false);

  // Load pages list from API
  const loadPages = async () => {
    try {
      setIsLoading(true);
      const res = await entriesApi.list('pages', { limit: 100, preview: true });
      setPages(Array.isArray(res.data) ? res.data : []);
    } catch (err: any) {
      toast.error('Error loading pages', err.message || 'Could not fetch pages list.');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadPages();
  }, []);

  // Auto-slug generation from title
  const handleTitleChange = (val: string) => {
    setNewTitle(val);
    const slugified = val
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '');
    setNewSlug(slugified);
  };

  // Handle Create Page
  const handleCreatePage = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newTitle.trim()) {
      toast.error('Title required', 'Please provide a title.');
      return;
    }
    const cleanSlug = newSlug.trim() || 'page-' + Date.now();

    try {
      setIsCreating(true);
      const res = await entriesApi.create('pages', {
        title: newTitle.trim(),
        slug: cleanSlug,
        template: newTemplate,
        blocks: [],
        seo: { meta_title: newTitle.trim() },
      });

      toast.success('Page Created', `Page "${newTitle}" created successfully.`);

      setIsCreateModalOpen(false);
      setNewTitle('');
      setNewSlug('');
      // Open straight into visual builder!
      onOpenBuilder(res.id, cleanSlug);
    } catch (err: any) {
      toast.error('Failed to create page', err.message || 'Could not save page record.');
    } finally {
      setIsCreating(false);
    }
  };

  // Delete page
  const handleDeletePage = async (id: string, title: string) => {
    if (!window.confirm(`Are you sure you want to delete the page "${title}"?`)) return;
    try {
      await entriesApi.delete('pages', id);
      setPages((prev) => prev.filter((p) => p.id !== id));
      toast.info('Page Deleted', `"${title}" has been removed.`);
    } catch (err: any) {
      toast.error('Delete Failed', err.message || 'Could not delete page.');
    }
  };

  // Publish / Unpublish directly from list
  const handleTogglePublish = async (page: ContentEntry) => {
    try {
      if (page.status === 'published') {
        await entriesApi.unpublish('pages', page.id);
        toast.info('Unpublished', 'Page returned to draft state.');
      } else {
        await entriesApi.publish('pages', page.id);
        toast.success('Published', 'Page is now live on the public site.');
      }
      loadPages();
    } catch (err: any) {
      toast.error('Status change failed', err.message);
    }
  };

  // Filter pages
  const filteredPages = pages.filter((p) => {
    const data = p.published_data || p.draft_data || {};
    const title = (data.title as string) || (p.slug as string) || '';
    const slug = (p.slug as string) || (data.slug as string) || '';
    const matchesSearch =
      title.toLowerCase().includes(searchTerm.toLowerCase()) ||
      slug.toLowerCase().includes(searchTerm.toLowerCase());
    const matchesStatus =
      statusFilter === 'all' || p.status === statusFilter;
    return matchesSearch && matchesStatus;
  });

  return (
    <div className="p-6 md:p-8 space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 pb-4 border-b border-slate-200">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <Globe className="w-5 h-5 text-sky-600" />
            <h1 className="text-xl font-bold text-slate-900 tracking-tight">Pages</h1>
          </div>
          <p className="text-xs text-slate-500">
            Create, design, and publish server-rendered HTML pages using the Visual Block Builder.
          </p>
        </div>
        <button
          onClick={() => setIsCreateModalOpen(true)}
          className="inline-flex items-center gap-2 px-4 py-2 bg-sky-600 hover:bg-sky-700 text-white font-semibold text-xs rounded-lg shadow-sm transition-colors cursor-pointer"
        >
          <Plus className="w-4 h-4" />
          <span>Create New Page</span>
        </button>
      </div>

      {/* Pages vs Articles Educational Banner */}
      <div className="bg-sky-50/80 border border-sky-200/80 rounded-xl p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs text-sky-950">
        <div className="flex items-start gap-3">
          <span className="text-lg leading-none mt-0.5">💡</span>
          <div>
            <p className="font-semibold text-sky-900">Understanding Pages vs. Articles in Go CMS</p>
            <p className="text-sky-700 mt-0.5">
              The <strong>Visual Page Builder</strong> manages standalone routable pages like your <strong>Home Page (/)</strong> and <strong>About Us</strong>.
              Structured blog posts, news, or articles are managed in <strong>Content Manager &rarr; Blog Posts</strong>. You can embed those articles into any page using the <em>Dynamic Posts</em> block!
            </p>
          </div>
        </div>
        {onNavigateToContent && (
          <button
            type="button"
            onClick={() => onNavigateToContent('blog-post')}
            className="shrink-0 px-3 py-1.5 bg-white hover:bg-sky-100/60 text-sky-700 font-semibold text-xs border border-sky-200 rounded-lg transition-colors cursor-pointer"
          >
            Go to Blog Posts &rarr;
          </button>
        )}
      </div>

      {/* Filter and Search Bar */}
      <div className="flex flex-col sm:flex-row gap-3 items-center justify-between">
        <div className="relative w-full sm:w-80">
          <Search className="w-4 h-4 absolute left-3 top-2.5 text-slate-400" />
          <input
            type="text"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            placeholder="Search pages by title or slug..."
            className="w-full pl-9 pr-3 py-1.5 text-xs bg-white border border-slate-200 rounded-lg focus:outline-none focus:ring-1 focus:ring-sky-500"
          />
        </div>

        <div className="flex gap-1 bg-slate-100 p-1 rounded-lg text-xs w-full sm:w-auto">
          {(['all', 'published', 'draft'] as const).map((st) => (
            <button
              key={st}
              onClick={() => setStatusFilter(st)}
              className={`px-3 py-1 rounded-md font-medium capitalize cursor-pointer transition-colors ${
                statusFilter === st
                  ? 'bg-white text-slate-900 shadow-xs'
                  : 'text-slate-500 hover:text-slate-900'
              }`}
            >
              {st}
            </button>
          ))}
        </div>
      </div>

      {/* Pages Table */}
      <div className="bg-white rounded-xl border border-slate-200 shadow-xs overflow-hidden">
        {isLoading ? (
          <div className="py-20 text-center text-xs text-slate-500 flex flex-col items-center gap-2">
            <div className="w-6 h-6 border-2 border-sky-600 border-t-transparent rounded-full animate-spin" />
            <span>Loading pages...</span>
          </div>
        ) : filteredPages.length === 0 ? (
          <div className="py-20 text-center flex flex-col items-center justify-center p-6">
            <div className="w-12 h-12 rounded-2xl bg-sky-50 text-sky-600 flex items-center justify-center mb-3">
              <FileText className="w-6 h-6" />
            </div>
            <h3 className="font-bold text-slate-800 text-sm mb-1">No pages found</h3>
            <p className="text-xs text-slate-500 max-w-sm mb-4">
              {searchTerm
                ? `No pages match your search "${searchTerm}".`
                : 'Get started by creating your first routable page with the Visual Block Builder.'}
            </p>
            <button
              onClick={() => setIsCreateModalOpen(true)}
              className="px-4 py-2 bg-sky-600 hover:bg-sky-700 text-white font-medium text-xs rounded-lg shadow-sm cursor-pointer transition-colors"
            >
              Create Page
            </button>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-xs">
              <thead>
                <tr className="bg-slate-50 border-b border-slate-200 text-slate-600 font-semibold uppercase tracking-wider text-[10px]">
                  <th className="py-3 px-4">Title & Route</th>
                  <th className="py-3 px-4">Template</th>
                  <th className="py-3 px-4">Status</th>
                  <th className="py-3 px-4">Version</th>
                  <th className="py-3 px-4 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {filteredPages.map((page) => {
                  const data = page.published_data || page.draft_data || {};
                  const title = (data.title as string) || page.slug || 'Untitled';
                  const slug = page.slug || (data.slug as string) || '';
                  const template = (data.template as string) || 'default';
                  const isPub = page.status === 'published';
                  const isHome = slug === 'home' || slug === '';

                  return (
                    <tr
                      key={page.id}
                      className="hover:bg-slate-50/70 transition-colors group"
                    >
                      <td className="py-3.5 px-4">
                        <div className="font-semibold text-slate-900 flex items-center gap-2 flex-wrap">
                          <FileText className={`w-4 h-4 ${isHome ? 'text-sky-600' : 'text-slate-400 group-hover:text-sky-600'} transition-colors`} />
                          <span>{title}</span>
                          {isHome && (
                            <span className="px-1.5 py-0.5 rounded text-[10px] font-bold bg-sky-100 text-sky-700 border border-sky-200">
                              Front Page (/)
                            </span>
                          )}
                        </div>
                        <div className="flex items-center gap-1 font-mono text-[11px] text-slate-400 mt-0.5">
                          <span>{isHome ? '/' : `/${slug}`}</span>
                          <a
                            href={isHome ? '/' : `/${slug}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            title={isHome ? 'Open live front page' : 'Open public page'}
                            className="hover:text-sky-600 transition-colors ml-1"
                          >
                            <ExternalLink className="w-3 h-3" />
                          </a>
                        </div>
                      </td>

                      <td className="py-3.5 px-4 text-slate-600 capitalize">
                        <span className="px-2 py-0.5 rounded bg-slate-100 text-slate-600 font-medium text-[11px]">
                          {template}
                        </span>
                      </td>

                      <td className="py-3.5 px-4">
                        <span
                          className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                            isPub
                              ? 'bg-emerald-50 text-emerald-700 border border-emerald-200'
                              : 'bg-amber-50 text-amber-700 border border-amber-200'
                          }`}
                        >
                          <span
                            className={`w-1.5 h-1.5 rounded-full ${
                              isPub ? 'bg-emerald-500' : 'bg-amber-500'
                            }`}
                          />
                          {page.status}
                        </span>
                      </td>

                      <td className="py-3.5 px-4 text-slate-500 font-mono text-[11px]">
                        v{page.draft_version || 1}
                      </td>

                      <td className="py-3.5 px-4 text-right">
                        <div className="flex items-center justify-end gap-2">
                          <button
                            onClick={() => onOpenBuilder(page.id, slug)}
                            className="px-2.5 py-1.5 bg-sky-50 hover:bg-sky-100 text-sky-700 font-medium rounded-md border border-sky-200 flex items-center gap-1 cursor-pointer transition-colors"
                          >
                            <Edit3 className="w-3.5 h-3.5" />
                            <span>Edit in Builder</span>
                          </button>

                          <button
                            onClick={() => handleTogglePublish(page)}
                            className={`px-2 py-1.5 rounded-md font-medium text-[11px] cursor-pointer transition-colors ${
                              isPub
                                ? 'bg-slate-100 hover:bg-slate-200 text-slate-700'
                                : 'bg-emerald-50 hover:bg-emerald-100 text-emerald-700 border border-emerald-200'
                            }`}
                          >
                            {isPub ? 'Unpublish' : 'Publish'}
                          </button>

                          <button
                            onClick={() => handleDeletePage(page.id, title)}
                            title="Delete Page"
                            className="p-1.5 text-slate-400 hover:text-rose-600 hover:bg-rose-50 rounded-md transition-colors cursor-pointer"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Create New Page Modal */}
      <Modal
        isOpen={isCreateModalOpen}
        onClose={() => setIsCreateModalOpen(false)}
        title="Create New Page"
      >
        <form onSubmit={handleCreatePage} className="space-y-4 text-xs">
          <div>
            <label className="block font-medium text-slate-700 mb-1">Page Title *</label>
            <input
              type="text"
              required
              value={newTitle}
              onChange={(e) => handleTitleChange(e.target.value)}
              placeholder="e.g. About Our Mission"
              className="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-lg focus:outline-none focus:ring-1 focus:ring-sky-500"
            />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="block font-medium text-slate-700">URL Route Slug *</label>
              <button
                type="button"
                onClick={() => {
                  setNewTitle('Home');
                  setNewSlug('home');
                }}
                className="text-[11px] text-sky-600 hover:text-sky-700 font-medium underline cursor-pointer"
              >
                Set as Front Page (slug: home)
              </button>
            </div>
            <div className="flex items-center">
              <span className="px-3 py-2 bg-slate-100 border border-r-0 border-slate-200 rounded-l-lg text-slate-500 font-mono text-xs">
                /
              </span>
              <input
                type="text"
                required
                value={newSlug}
                onChange={(e) => setNewSlug(e.target.value.toLowerCase().replace(/[^a-z0-9-_]/g, ''))}
                placeholder="about-our-mission"
                className="flex-1 px-3 py-2 bg-slate-50 border border-slate-200 rounded-r-lg font-mono text-xs focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
            <p className="text-[11px] text-slate-400 mt-1">
              This page will be accessible at http://localhost:8080/{newSlug === 'home' ? '' : (newSlug || 'slug')}.
            </p>
          </div>

          <div>
            <label className="block font-medium text-slate-700 mb-1">Theme Layout Template</label>
            <select
              value={newTemplate}
              onChange={(e) =>
                setNewTemplate(e.target.value as 'default' | 'full-width' | 'landing')
              }
              className="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-lg text-xs"
            >
              <option value="default">Default Template (Contained)</option>
              <option value="full-width">Full Width (Edge-to-Edge)</option>
              <option value="landing">Landing Page</option>
            </select>
          </div>

          <div className="pt-3 border-t border-slate-100 flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => setIsCreateModalOpen(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={isCreating}>
              {isCreating ? 'Creating...' : 'Create & Open Builder'}
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
