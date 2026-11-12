import React from 'react';
import { TokenManagerPanel } from '../../components/settings/TokenManagerPanel';
import type { User } from '../../types';

export interface TransferTokensViewProps {
  currentUser: User;
}

export const TransferTokensView: React.FC<TransferTokensViewProps> = ({ currentUser }) => {
  return <TokenManagerPanel mode="transfer" currentUser={currentUser} />;
};

