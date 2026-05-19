import React from 'react';
import { FileCode2, FolderOpen, Search } from 'lucide-react';
import { AnchoredPopover, Input } from '../ui';

export default function FileSearchPopover({ ide, anchorRef }) {
  const inputRef = React.useRef(null);

  React.useEffect(() => {
    if (!ide.isFileSearchOpen) return undefined;
    inputRef.current?.focus();
    return undefined;
  }, [ide.isFileSearchOpen]);

  const query = ide.fileSearchQuery || '';
  const hasQuery = query.trim().length > 0;
  const resultCount = ide.fileSearchResults?.length || 0;

  return (
    <AnchoredPopover
      anchorRef={anchorRef}
      width={360}
      minWidth={220}
      open={ide.isFileSearchOpen}
      onClose={ide.closeFileSearch}
      testId="workspace-search-popover"
    >
      {({ position }) => (
        <>
          <div className="border-b border-gray-100 p-3 dark:border-white/10">
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-blue-50 text-blue-500 dark:bg-blue-500/10 dark:text-blue-300">
                <Search size={15} />
              </div>
              <Input
                ref={inputRef}
                data-testid="workspace-search-input"
                value={query}
                onChange={(event) => ide.setFileSearchQuery(event.target.value)}
                className="h-9 border-transparent bg-gray-100/80 text-[13px] dark:bg-white/[0.07]"
                placeholder="搜索文件或文件夹"
              />
            </div>
          </div>
          <div
            className="overflow-y-auto p-1.5 custom-scrollbar scrollbar-rounded-clip"
            style={{ maxHeight: Math.max(132, position.maxHeight - 66) }}
          >
            {ide.fileSearchError ? (
              <div className="px-3 py-5 text-center text-sm text-red-500 dark:text-red-300">{ide.fileSearchError}</div>
            ) : ide.isFileSearchLoading ? (
              <div className="px-3 py-5 text-center text-sm text-gray-500 dark:text-gray-400">正在搜索...</div>
            ) : !hasQuery ? (
              <div className="px-3 py-5 text-center text-sm text-gray-500 dark:text-gray-400">输入文件名或路径开始搜索。</div>
            ) : resultCount === 0 ? (
              <div className="px-3 py-5 text-center text-sm text-gray-500 dark:text-gray-400">没有找到匹配的文件。</div>
            ) : (
              ide.fileSearchResults.map((item) => (
                <button
                  key={item.path}
                  type="button"
                  data-testid={`workspace-search-result-${item.path}`}
                  onClick={() => ide.openSearchResult(item).catch((error) => console.error(error))}
                  className="motion-row flex w-full min-w-0 items-center gap-2 rounded-xl px-3 py-2.5 text-left text-gray-700 hover:bg-blue-50/90 hover:text-blue-600 dark:text-gray-200 dark:hover:bg-blue-500/15 dark:hover:text-blue-200"
                >
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-gray-100 text-gray-500 dark:bg-white/[0.08] dark:text-gray-300">
                    {item.type === 'folder' ? <FolderOpen size={14} /> : <FileCode2 size={14} />}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-semibold">{item.name || item.path}</span>
                    <span className="mt-0.5 block truncate text-[11px] text-gray-400">{item.path}</span>
                  </span>
                </button>
              ))
            )}
          </div>
        </>
      )}
    </AnchoredPopover>
  );
}
