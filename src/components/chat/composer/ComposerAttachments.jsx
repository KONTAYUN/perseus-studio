import React from 'react';
import { File, X } from 'lucide-react';
import { IconButton } from '../../ui';
import { formatFileSize } from '../chatUtils';

export default function ComposerAttachments({ attachments, onRemove }) {
  if (attachments.length === 0) return null;

  return (
    <div className="composer-attachments">
      {attachments.map((item) => (
        <div
          key={item.id}
          className="motion-chip composer-attachment-chip"
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
