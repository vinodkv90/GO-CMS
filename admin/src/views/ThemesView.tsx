import React from 'react';
import { Palette, ExternalLink, CheckCircle, Code, Layers, FileCode } from 'lucide-react';
import { Card, CardContent } from '../components/ui/Card';
import type { ThemeManifest } from '../types';

export interface ThemesViewProps {
  themes: ThemeManifest[];
}

export const ThemesView: React.FC<ThemesViewProps> = ({ themes }) => {
  return (
    <div className="space-y-6">
      {/* Top Header */}
      <div>
        <h1 className="text-2xl font-bold text-slate-900 tracking-tight">Theme Management</h1>
        <p className="text-xs text-slate-500 mt-1">
          Server-side template themes powered exclusively by Go standard library{' '}
          <code className="font-mono text-slate-700">html/template</code>, safe HTML sanitization, and layouts.
        </p>
      </div>

      {/* Themes Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {themes.length === 0 ? (
          <div className="col-span-full">
            <Card>
              <CardContent className="p-12 text-center text-slate-400">
                <Palette className="w-12 h-12 mx-auto mb-3 text-slate-300" />
                <h3 className="text-base font-bold text-slate-800">No themes found</h3>
                <p className="text-xs text-slate-500 mt-1">
                  Ensure themes exist in the <code className="font-mono text-slate-700">themes/</code> directory.
                </p>
              </CardContent>
            </Card>
          </div>
        ) : (
          themes.map((theme) => {
            const isActive = Boolean(theme.is_active || theme.slug === 'default');

            return (
              <Card key={theme.slug} className="flex flex-col justify-between border-slate-200 overflow-hidden">
                <div>
                  {/* Theme Preview Header */}
                  <div className="h-40 bg-gradient-to-tr from-slate-900 via-sky-950 to-indigo-900 p-6 flex flex-col justify-between relative overflow-hidden">
                    <div className="flex items-center justify-between relative z-10">
                      <span className="font-mono text-xs px-2 py-0.5 rounded bg-white/10 text-white border border-white/10">
                        {theme.slug}
                      </span>
                      {isActive && (
                        <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-bold bg-emerald-400/20 text-emerald-300 border border-emerald-400/30">
                          <CheckCircle className="w-3.5 h-3.5" />
                          <span>Active Theme</span>
                        </span>
                      )}
                    </div>
                    <div className="relative z-10">
                      <h3 className="text-xl font-black text-white">{theme.name}</h3>
                      <p className="text-xs text-sky-200 mt-0.5">Version {theme.version}</p>
                    </div>
                  </div>

                  <CardContent className="p-5 space-y-4">
                    <p className="text-xs text-slate-600 leading-relaxed">
                      {theme.description || 'Native Go CMS production theme.'}
                    </p>

                    <div className="space-y-2 pt-2 border-t border-slate-100 text-xs text-slate-500">
                      <div className="flex items-center gap-2">
                        <Code className="w-3.5 h-3.5" />
                        <span>Engine:</span>
                        <span className="font-mono font-semibold text-slate-700">{theme.engine || 'html/template'}</span>
                      </div>
                      <div className="flex items-center gap-2">
                        <Layers className="w-3.5 h-3.5" />
                        <span>Author:</span>
                        <span className="text-slate-700">{theme.author || 'Go CMS Team'}</span>
                      </div>
                    </div>

                    <div className="p-3 rounded-lg bg-slate-50 border border-slate-200/80 text-[11px] text-slate-600 space-y-1">
                      <div className="font-semibold text-slate-800 flex items-center gap-1.5">
                        <FileCode className="w-3.5 h-3.5 text-sky-600" />
                        <span>Template Hierarchy:</span>
                      </div>
                      <div className="font-mono text-[10px] text-slate-500 pl-5">
                        themes/{theme.slug}/templates/layouts/base.html<br />
                        themes/{theme.slug}/templates/pages/*.html<br />
                        themes/{theme.slug}/templates/partials/*.html
                      </div>
                    </div>
                  </CardContent>
                </div>

                <div className="p-4 bg-slate-50 border-t border-slate-100 flex items-center justify-between">
                  <a
                    href="/"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1.5 text-xs font-semibold text-sky-600 hover:text-sky-700"
                  >
                    <span>Preview Site</span>
                    <ExternalLink className="w-3.5 h-3.5" />
                  </a>

                  <a
                    href="/?preview=true"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1.5 text-xs font-semibold text-slate-600 hover:text-slate-800"
                  >
                    <span>Draft Preview (?preview=true)</span>
                  </a>
                </div>
              </Card>
            );
          })
        )}
      </div>
    </div>
  );
};
