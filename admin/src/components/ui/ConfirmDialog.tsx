import React from 'react';
import { AlertTriangle } from 'lucide-react';
import { Modal } from './Modal';
import { Button } from './Button';

export interface ConfirmDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => void | Promise<void>;
  title: string;
  description: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: 'destructive' | 'primary';
  isLoading?: boolean;
}

export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  isOpen,
  onClose,
  onConfirm,
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  variant = 'destructive',
  isLoading = false,
}) => {
  return (
    <Modal isOpen={isOpen} onClose={() => { if (!isLoading) onClose(); }} title={title} maxWidth="sm">
      <div className="space-y-4">
        <div className="flex items-start gap-3">
          <div
            className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full ${
              variant === 'destructive' ? 'bg-rose-100 text-rose-600' : 'bg-sky-100 text-sky-600'
            }`}
          >
            <AlertTriangle className="h-4 w-4" />
          </div>
          <p className="text-sm text-slate-600 leading-relaxed">{description}</p>
        </div>

        <div className="flex justify-end gap-2.5 border-t border-slate-100 pt-4">
          <Button type="button" variant="outline" size="sm" disabled={isLoading} onClick={onClose}>
            {cancelLabel}
          </Button>
          <Button
            type="button"
            variant={variant}
            size="sm"
            isLoading={isLoading}
            onClick={() => void onConfirm()}
          >
            {confirmLabel}
          </Button>
        </div>
      </div>
    </Modal>
  );
};

