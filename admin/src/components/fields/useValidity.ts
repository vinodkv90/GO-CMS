import { useEffect, useRef } from 'react';
import type { ValidityChange } from './contracts';

export function useValidity(path: string, message: string | undefined, callback?: ValidityChange) {
  const latest = useRef(callback);
  latest.current = callback;
  useEffect(() => {
    latest.current?.(path, message || null);
    return () => latest.current?.(path, null);
  }, [path, message, Boolean(callback)]);
}
