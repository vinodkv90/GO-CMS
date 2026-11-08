import React from 'react';

export interface BadgeProps {
  children: React.ReactNode;
  variant?: 'draft' | 'published' | 'archived' | 'collection' | 'single' | 'success' | 'warning' | 'info' | 'default';
  className?: string;
}

export const Badge: React.FC<BadgeProps> = ({ children, variant = 'default', className = '' }) => {
  const variantStyles = {
    draft: 'bg-amber-100 text-amber-800 border-amber-200',
    published: 'bg-emerald-100 text-emerald-800 border-emerald-200',
    archived: 'bg-slate-100 text-slate-700 border-slate-200',
    collection: 'bg-sky-100 text-sky-800 border-sky-200',
    single: 'bg-purple-100 text-purple-800 border-purple-200',
    success: 'bg-emerald-100 text-emerald-800 border-emerald-200',
    warning: 'bg-amber-100 text-amber-800 border-amber-200',
    info: 'bg-blue-100 text-blue-800 border-blue-200',
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

