import React from 'react';
import { Download } from 'lucide-react';
import { API_BASE_URL } from '../../../api/client';
import { isNotePath } from '../../../hooks/noteTreeUtils.js';
import { AnchoredPopover, Button } from '../../ui';

function triggerBrowserDownload(path) {
  const anchor = document.createElement('a');
  anchor.href = `${API_BASE_URL}${path}`;
  anchor.download = '';
  anchor.rel = 'noopener';
  anchor.style.display = 'none';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
}

export default function HistoryExportButton({ selectedItem, selectedPath, disabled }) {
  const anchorRef = React.useRef(null);
  const [isOpen, setIsOpen] = React.useState(false);
  const canExport = Boolean(selectedItem?.id && selectedPath && isNotePath(selectedPath));

  const exportVersion = (format) => {
    if (!canExport) return;
    const params = new URLSearchParams();
    params.set('version_id', selectedItem.id);
    params.set('path', selectedPath);
    params.set('format', format);
    triggerBrowserDownload(`/api/files/export?${params.toString()}`);
  };

  return (
    <>
      <Button
        ref={anchorRef}
        variant="subtle"
        size="sm"
        disabled={disabled || !canExport}
        onClick={() => setIsOpen((current) => !current)}
        title={canExport ? '导出历史记录' : '只有 HTML 笔记历史支持导出'}
      >
        <Download size={14} />
        导出
      </Button>
      <AnchoredPopover
        anchorRef={anchorRef}
        width={176}
        minWidth={176}
        maxHeight={220}
        open={isOpen && canExport}
        onClose={() => setIsOpen(false)}
        testId="history-export-menu"
        className="py-1"
      >
        {[
          ['html', 'HTML (.html)'],
          ['md', 'Markdown (.md)'],
          ['docx', 'Word (.docx)'],
        ].map(([format, label]) => (
          <button
            key={format}
            type="button"
            className="motion-row flex w-full items-center px-3 py-2 text-left text-sm text-gray-700 hover:bg-blue-50 hover:text-blue-600 dark:text-gray-300 dark:hover:bg-blue-500/20"
            onClick={(event) => {
              event.stopPropagation();
              setIsOpen(false);
              exportVersion(format);
            }}
          >
            {label}
          </button>
        ))}
      </AnchoredPopover>
    </>
  );
}
