import React, { useState } from 'react';
import { Check, Copy, ShieldAlert } from 'lucide-react';
import { Button } from './Button';

export interface CopyableSecretProps {
  title?: string;
  description?: string;
  secret: string;
  label?: string;
}

export const CopyableSecret: React.FC<CopyableSecretProps> = ({
  title = 'Save your secret now',
  description = 'For your security, this plaintext value will not be shown again once you close this dialog.',
  secret,
  label = 'Token / Secret',
}) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  return (
    <div className="rounded-xl border border-amber-200 bg-amber-50/70 p-4 space-y-3">
      <div className="flex items-start gap-2.5 text-amber-900">
        <ShieldAlert className="w-4 h-4 shrink-0 mt-0.5 text-amber-600" />
        <div>
          <p className="text-sm font-semibold">{title}</p>
          <p className="text-xs text-amber-800 mt-0.5">{description}</p>
        </div>
      </div>

      <div className="space-y-1">
        <p className="text-[11px] font-semibold uppercase tracking-wider text-amber-900/80">{label}</p>
        <div className="flex items-center gap-2 bg-white border border-amber-200 rounded-lg p-2">
          <code className="flex-1 text-xs font-mono text-slate-900 break-all select-all">
            {secret}
          </code>
          <Button type="button" variant="outline" size="sm" onClick={() => void handleCopy()}>
            {copied ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
            <span>{copied ? 'Copied' : 'Copy'}</span>
          </Button>
        </div>
      </div>
    </div>
  );
};

