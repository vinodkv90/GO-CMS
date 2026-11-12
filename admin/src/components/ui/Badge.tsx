import React from 'react';

export interface BadgeProps {
  children: React.ReactNode;
  variant?:
    | 'draft'
    | 'published'
    | 'modified'
    | 'archived'
    | 'collection'
    | 'single'
    | 'component'
    | 'success'
    | 'warning'
    | 'error'
    | 'info'
    | 'neutral'
    | 'default';
  className?: string;
}

export const Badge: React.FC<BadgeProps> = ({ children, variant = 'default', className = '' }) => {
  const variantStyles = {
    draft: 'bg-sky-50 text-sky-700 border-sky-200',
    published: 'bg-emerald-50 text-emerald-700 border-emerald-200',
    modified: 'bg-violet-50 text-violet-700 border-violet-200',
    archived: 'bg-slate-100 text-slate-700 border-slate-200',
    collection: 'bg-sky-100 text-sky-800 border-sky-200',
    single: 'bg-purple-100 text-purple-800 border-purple-200',
    component: 'bg-indigo-100 text-indigo-800 border-indigo-200',
    success: 'bg-emerald-100 text-emerald-800 border-emerald-200',
    warning: 'bg-amber-100 text-amber-800 border-amber-200',
    error: 'bg-rose-100 text-rose-800 border-rose-200',
    info: 'bg-blue-100 text-blue-800 border-blue-200',
    neutral: 'bg-slate-100 text-slate-600 border-slate-200',
    default: 'bg-slate-100 text-slate-800 border-slate-200',
  };

  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold border capitalize ${variantStyles[variant]} ${className}`}
    >
      {children}
    </span>
  );
};
