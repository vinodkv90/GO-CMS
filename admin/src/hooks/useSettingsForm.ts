import { useCallback, useEffect, useMemo, useState } from 'react';
import { useToast } from '../components/ui/Toast';

export interface SettingsFormHandlers {
  save: () => Promise<void>;
  discard: () => void;
  isSaving: boolean;
}

export interface UseSettingsFormOptions<T> {
  load: () => Promise<T>;
  save: (value: T) => Promise<T>;
  validate?: (value: T) => string | null;
  successTitle: string;
  successMessage: string;
  loadErrorFallback: string;
  saveErrorFallback: string;
  onDirtyChange: (dirty: boolean) => void;
  registerHandlers: (handlers: SettingsFormHandlers) => void;
}

export function useSettingsForm<T>({
  load,
  save,
  validate,
  successTitle,
  successMessage,
  loadErrorFallback,
  saveErrorFallback,
  onDirtyChange,
  registerHandlers,
}: UseSettingsFormOptions<T>) {
  const toast = useToast();
  const [baseline, setBaseline] = useState<T | null>(null);
  const [form, setForm] = useState<T | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      setIsLoading(true);
      setError(null);
      try {
        const data = await load();
        if (!cancelled) {
          setBaseline(data);
          setForm(data);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error && err.message ? err.message : loadErrorFallback);
        }
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [load, loadErrorFallback]);

  const isDirty = useMemo(() => {
    if (!baseline || !form) return false;
    return JSON.stringify(baseline) !== JSON.stringify(form);
  }, [baseline, form]);

  useEffect(() => {
    onDirtyChange(isDirty);
  }, [isDirty, onDirtyChange]);

  const handleDiscard = useCallback(() => {
    if (baseline) {
      setForm(baseline);
      setError(null);
    }
  }, [baseline]);

  const handleSave = useCallback(async () => {
    if (!form) return;
    if (validate) {
      const msg = validate(form);
      if (msg) {
        setError(msg);
        return;
      }
    }
    setIsSaving(true);
    setError(null);
    try {
      const saved = await save(form);
      setBaseline(saved);
      setForm(saved);
      toast.success(successTitle, successMessage);
    } catch (err) {
      const message = err instanceof Error && err.message ? err.message : saveErrorFallback;
      setError(message);
      toast.error('Failed to save settings', message);
    } finally {
      setIsSaving(false);
    }
  }, [form, save, saveErrorFallback, successMessage, successTitle, toast, validate]);

  useEffect(() => {
    registerHandlers({
      save: handleSave,
      discard: handleDiscard,
      isSaving,
    });
  }, [handleDiscard, handleSave, isSaving, registerHandlers]);

  return {
    form,
    setForm,
    isLoading,
    isSaving,
    isDirty,
    error,
    setError,
    handleSave,
    handleDiscard,
  };
}

