import React from 'react';
import { AlertCircle, File, LoaderCircle, RotateCcw, X } from 'lucide-react';
import { IconButton } from '../../ui';
import { formatFileSize } from '../chatUtils';

export default function ComposerAttachments({ attachments, onRemove, onRetry }) {
  if (attachments.length === 0) return null;

  return (
    <div className="composer-attachments">
      {attachments.map((item) => (
        <div
          key={item.id}
          className={`motion-chip composer-attachment-chip ${item.kind === 'image' ? 'composer-image-attachment' : ''}`}
        >
          {item.kind === 'image' ? (
            <img
              src={item.preview_url || item.content_url}
              alt=""
              className="h-8 w-8 shrink-0 rounded object-cover"
            />
          ) : (
            <File size={13} className="shrink-0 text-blue-500" />
          )}
          <span className="max-w-[180px] truncate">{item.name}</span>
          <span className="shrink-0 text-[10px] text-gray-400">{formatFileSize(item.size)}</span>
          {item.upload_status === 'uploading' && <LoaderCircle size={13} className="shrink-0 animate-spin text-blue-500" />}
          {item.upload_status === 'failed' && (
            <>
              <AlertCircle size={13} className="shrink-0 text-red-500" />
              <IconButton
                size="compact"
                onClick={(event) => {
                  event.stopPropagation();
                  onRetry?.(item.id);
                }}
                title="重试上传"
              >
                <RotateCcw size={12} />
              </IconButton>
            </>
          )}
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
