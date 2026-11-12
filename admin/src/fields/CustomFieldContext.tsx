import React, { createContext, useContext } from 'react';
import { useFieldCapabilities } from './useFieldCapabilities';
import type { CustomFieldDescriptor } from './useFieldCapabilities';

const CustomFieldsContext = createContext<CustomFieldDescriptor[]>([]);

export const CustomFieldsProvider: React.FC<{
  value?: CustomFieldDescriptor[];
  children: React.ReactNode;
}> = ({ value, children }) => {
  const { customFields } = useFieldCapabilities();
  return (
    <CustomFieldsContext.Provider value={value ?? customFields}>
      {children}
    </CustomFieldsContext.Provider>
  );
};

export function useRegisteredCustomFields(): CustomFieldDescriptor[] {
  return useContext(CustomFieldsContext);
}

