import { useEffect, useState } from 'react';
import { api, ApiError } from '../api/client';
import type { FieldCapabilityMap } from './registry';

export interface CustomFieldDescriptor {
  type: string;
  label?: string;
  description?: string;
  plugin_slug?: string;
  plugin_name?: string;
  settings?: string[];
}

export function useFieldCapabilities() {
  const [capabilities, setCapabilities] = useState<FieldCapabilityMap>();
  const [customFields, setCustomFields] = useState<CustomFieldDescriptor[]>([]);
  const [error, setError] = useState<string>();

  useEffect(() => {
    const controller = new AbortController();

    api
      .get<FieldCapabilityMap>('/content-types/field-capabilities', { signal: controller.signal })
      .then((response) => {
        if (
          !response ||
          typeof response !== 'object' ||
          !Object.keys(response).length ||
          !Object.values(response).every(
            (entry) =>
              entry &&
              Array.isArray(entry.settings) &&
              entry.settings.every((key: unknown) => typeof key === 'string'),
          )
        ) {
          throw new Error('Invalid field capabilities response.');
        }
        if (!controller.signal.aborted) setCapabilities(response);
      })
      .catch((reason: unknown) => {
        if (controller.signal.aborted) return;
        // Older servers do not expose capabilities; the local registry remains usable.
        if (reason instanceof ApiError && reason.status === 404) return;
        setError('Field support could not be verified. Some settings may be rejected when saved.');
      });

    api
      .get<{ data?: CustomFieldDescriptor[] } | CustomFieldDescriptor[]>(
        '/content-types/custom-fields',
        { signal: controller.signal },
      )
      .then((res) => {
        if (controller.signal.aborted) return;
        const list = Array.isArray(res) ? res : Array.isArray(res?.data) ? res.data : [];
        setCustomFields(list.filter((item) => item && typeof item.type === 'string'));
      })
      .catch(() => {
        // Older servers without custom-fields endpoint gracefully show empty custom fields.
      });

    return () => controller.abort();
  }, []);

  return { capabilities, customFields, error };
}
