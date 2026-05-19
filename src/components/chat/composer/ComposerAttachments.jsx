import React from 'react';
import { File, X } from 'lucide-react';
import { IconButton } from '../../ui';
import { formatFileSize } from '../chatUtils';

export default function ComposerAttachments({ attachments, onRemove }) {
  if (attachments.length === 0) return null;

  return (
    <div className="mb-2 flex flex-wrap gap-2 px-1.5 pt-0.5">
      {attachments.map((item) => (
        <div
          key={item.id}
          className="motion-chip flex max-w-full items-center gap-2 rounded-2xl border border-gray-200/70 bg-gray-50/70 px-3 py-1.5 text-xs text-gray-600 dark:border-white/10 dark:bg-white/[0.05] dark:text-gray-300"
        >
          <File size={13} className="shrink-0 text-blue-500" />
          <span className="max-w-[180px] truncate">{item.name}</span>
          <span className="shrink-0 text-[10px] text-gray-400">{formatFileSize(item.size)}</span>
          <IconButton
            size="compact"
            variant="danger"
            onClick={(e) => {
              e.stopPropagation();
              onRemove(item.id);
            }}
            title="移除附件"
          >
            <X size={12} />
          </IconButton>
        </div>
      ))}
    </div>
  );
}
