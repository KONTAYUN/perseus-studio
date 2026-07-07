import React from 'react';
import { Download } from 'lucide-react';
import { AnchoredPopover } from '../ui';

export default function NoteTreeExportButton({ item, ide }) {
  const anchorRef = React.useRef(null);
  const [isOpen, setIsOpen] = React.useState(false);

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        data-testid={`download-${item.path}`}
        onMouseDown={(e) => e.stopPropagation()}
        onClick={(e) => {
          e.stopPropagation();
          ide.selectItem?.(item.path, item.type);
          setIsOpen((current) => !current);
        }}
        className="motion-button ui-icon-button ui-icon-button-compact h-5 w-5"
        title="导出笔记"
      >
        <Download size={12} />
      </button>
      <AnchoredPopover
        anchorRef={anchorRef}
        width={176}
        minWidth={176}
        maxHeight={220}
        open={isOpen}
        onClose={() => setIsOpen(false)}
        testId={`note-tree-export-menu-${item.path}`}
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
              ide.exportNotes?.({
                path: item.path,
                type: 'file',
                format,
              }).catch((error) => console.error(error));
            }}
          >
            {label}
          </button>
        ))}
      </AnchoredPopover>
    </>
  );
}
