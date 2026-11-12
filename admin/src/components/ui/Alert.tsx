import React from 'react';
import { AlertCircle, AlertTriangle, CheckCircle2, Info } from 'lucide-react';
import { cn } from '../../lib/cn';

export interface AlertProps {
  variant?: 'info' | 'success' | 'warning' | 'error';
  title?: string;
  children: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
}

const variantClasses = {
  info: 'bg-sky-50/80 border-sky-200 text-sky-900',
  success: 'bg-emerald-50/80 border-emerald-200 text-emerald-900',
  warning: 'bg-amber-50/80 border-amber-200 text-amber-900',
  error: 'bg-rose-50/80 border-rose-200 text-rose-900',
};

const iconMap = {
  info: Info,
  success: CheckCircle2,
  warning: AlertTriangle,
  error: AlertCircle,
};

const iconTone = {
  info: 'text-sky-600',
  success: 'text-emerald-600',
  warning: 'text-amber-600',
  error: 'text-rose-600',
};

export const Alert: React.FC<AlertProps> = ({
  variant = 'info',
  title,
  children,
  action,
  className = '',
}) => {
  const Icon = iconMap[variant];

  return (
    <div
      role={variant === 'error' || variant === 'warning' ? 'alert' : 'status'}
      className={cn(
        'rounded-xl border px-4 py-3 flex items-start justify-between gap-3 text-xs leading-relaxed',
        variantClasses[variant],
        className,
      )}
    >
      <div className="flex items-start gap-2.5">
        <Icon className={cn('w-4 h-4 shrink-0 mt-0.5', iconTone[variant])} />
        <div className="space-y-0.5">
          {title && <p className="font-semibold text-sm">{title}</p>}
          <div>{children}</div>
        </div>
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
};

