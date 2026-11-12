import React from 'react';
import { cn } from '../../lib/cn';

export const Skeleton: React.FC<{ className?: string }> = ({ className = '' }) => (
  <div className={cn('animate-pulse rounded-md bg-slate-200/80', className)} />
);

export const SettingsCardSkeleton: React.FC<{ rows?: number }> = ({ rows = 4 }) => (
  <div className="bg-white rounded-xl border border-slate-200/80 p-6 space-y-5">
    <div className="space-y-2 border-b border-slate-100 pb-4">
      <Skeleton className="h-5 w-48" />
      <Skeleton className="h-3.5 w-80" />
    </div>
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="space-y-2">
          <Skeleton className="h-3.5 w-28" />
          <Skeleton className="h-9 w-full" />
        </div>
      ))}
    </div>
  </div>
);

