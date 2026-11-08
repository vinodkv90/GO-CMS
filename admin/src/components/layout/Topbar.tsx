import React from 'react';
import { ExternalLink, ShieldCheck, User as UserIcon } from 'lucide-react';
import type { User } from '../../types';

export interface TopbarProps {
  user: User | null;
  pageTitle: string;
}

export const Topbar: React.FC<TopbarProps> = ({ user, pageTitle }) => {
  const roleName = user?.roles?.[0]?.name || user?.roles?.[0]?.slug || 'Administrator';

  return (
    <header className="h-16 bg-white border-b border-slate-200/80 px-8 flex items-center justify-between shrink-0 select-none">
      <div className="flex items-center gap-3">
        <h2 className="text-lg font-bold text-slate-800 tracking-tight">{pageTitle}</h2>
        <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-sky-50 text-sky-700 border border-sky-100 uppercase tracking-wider">
          v1.0.0
        </span>
      </div>

      <div className="flex items-center gap-4">
        {/* View Public Website */}
        <a
          href="/"
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-slate-200 text-xs font-semibold text-slate-600 hover:text-sky-600 hover:border-sky-200 transition-colors"
        >
          <span>View Site</span>
          <ExternalLink className="w-3.5 h-3.5" />
        </a>

        <div className="h-4 w-px bg-slate-200" />

        {/* User Badge */}
        <div className="flex items-center gap-2.5">
          <div className="w-8 h-8 rounded-full bg-slate-100 border border-slate-200 flex items-center justify-center text-slate-600">
            <UserIcon className="w-4 h-4" />
          </div>
          <div className="text-left">
            <p className="text-xs font-bold text-slate-800 leading-none">{user?.username || 'Admin'}</p>
            <div className="flex items-center gap-1 mt-1 text-[10px] text-slate-500 font-medium">
              <ShieldCheck className="w-3 h-3 text-emerald-600" />
              <span className="capitalize">{roleName}</span>
            </div>
          </div>
        </div>
      </div>
    </header>
  );
};

