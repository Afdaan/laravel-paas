/* eslint-disable react-refresh/only-export-components */
import React, { createContext, useContext } from 'react'
import { DatabaseInstance, DatabaseBackup } from '../../types'

export interface ConfirmationModalOptions {
  title: string;
  message: string;
  type: 'danger' | 'warning' | 'info';
  confirmText?: string;
  cancelText?: string;
  onConfirm: () => void;
}

export interface SchemaColumn {
  name: string;
  type: string;
  key?: string;
  extra?: string;
  nullable: boolean | string;
  default?: string | number | boolean | null;
  null?: string;
}

export interface SchemaTable {
  name: string;
  columns: SchemaColumn[];
  rows?: number;
  size?: string;
}

export interface StudioContextType {
  id: string | number;
  dbOverview: DatabaseInstance | null;
  schemaData: SchemaTable[];
  backups: DatabaseBackup[];
  metrics: Record<string, unknown> | null;
  isActionLoading: boolean;
  setIsActionLoading: (val: boolean) => void;
  loadStudioData: () => Promise<void>;
  triggerConfirmation: (options: ConfirmationModalOptions) => void;
  setActiveTab: (tab: 'dashboard' | 'tables' | 'structure' | 'query' | 'backups') => void;
  t: (keyPath: string, data?: Record<string, string | number>) => string;
}

const StudioContext = createContext<StudioContextType | undefined>(undefined)

interface StudioProviderProps {
  children: React.ReactNode;
  value: StudioContextType;
}

export function StudioProvider({ children, value }: StudioProviderProps) {
  return <StudioContext.Provider value={value}>{children}</StudioContext.Provider>
}

export function useStudio() {
  const context = useContext(StudioContext)
  if (context === undefined) {
    throw new Error('useStudio must be used within a StudioProvider')
  }
  return context
}
