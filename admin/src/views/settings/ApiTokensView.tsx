import React from 'react';
import { TokenManagerPanel } from '../../components/settings/TokenManagerPanel';
import type { User } from '../../types';

export interface ApiTokensViewProps {
  currentUser: User;
}

export const ApiTokensView: React.FC<ApiTokensViewProps> = ({ currentUser }) => {
  return <TokenManagerPanel mode="api" currentUser={currentUser} />;
};

