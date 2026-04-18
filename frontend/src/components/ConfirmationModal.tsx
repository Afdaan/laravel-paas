import React from 'react'
import { AlertTriangle, Info, AlertOctagon } from 'lucide-react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

interface ConfirmationModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: string;
  message: React.ReactNode;
  type?: 'danger' | 'warning' | 'info';
  confirmText?: string;
  cancelText?: string;
}

export default function ConfirmationModal({ 
  isOpen, onClose, onConfirm, title, message, type = 'danger', confirmText = 'Confirm', cancelText = 'Cancel' 
}: ConfirmationModalProps) {
  const configs = {
    danger: {
      text: 'text-destructive',
      btn: 'bg-destructive hover:bg-destructive/90 text-destructive-foreground',
      icon: AlertOctagon
    },
    warning: {
      text: 'text-amber-500',
      btn: 'bg-primary hover:bg-primary/90 text-primary-foreground',
      icon: AlertTriangle
    },
    info: {
      text: 'text-blue-500',
      btn: 'bg-primary hover:bg-primary/90 text-primary-foreground',
      icon: Info
    }
  }

  const config = configs[type] || configs.danger
  const Icon = config.icon

  React.useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        onConfirm();
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onConfirm, onClose]);

  return (
    <AlertDialog open={isOpen} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <div className="flex items-center gap-3">
            <Icon className={`w-6 h-6 ${config.text}`} />
            <AlertDialogTitle>{title}</AlertDialogTitle>
          </div>
          <AlertDialogDescription>
            {message}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel onClick={onClose}>{cancelText}</AlertDialogCancel>
          <AlertDialogAction 
            className={config.btn}
            onClick={(e) => {
              e.preventDefault();
              onConfirm();
              onClose();
            }}
          >
            {confirmText}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

