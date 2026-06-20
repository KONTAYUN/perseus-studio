import React from 'react';
import { createPortal } from 'react-dom';
import { Captions, ChevronDown, Columns3, Combine, Rows3, Trash2 } from 'lucide-react';
import { Button, IconButton, Input } from '../../ui';

function TableToolbarButton({ active = false, children, ...props }) {
  return (
    <IconButton size="xs" active={active} className="h-8 w-8 shrink-0" {...props}>
      {children}
    </IconButton>
  );
}

function TableDropdown({ children, label, onToggle, open }) {
  return (
    <button
      type="button"
      title={label}
      onMouseDown={(event) => event.preventDefault()}
      onClick={onToggle}
      className={`motion-button rich-text-table-trigger inline-flex h-8 items-center gap-1 rounded-xl px-2 text-sm font-semibold ${
        open ? 'rich-text-table-trigger-active' : ''
      }`}
    >
      {children}
      <ChevronDown size={12} />
    </button>
  );
}

const COLUMN_ACTIONS = [
  { key: 'toggleHeaderColumn', label: '标题列', command: 'toggleHeaderColumn' },
  { key: 'addColumnBefore', label: '向左插入一列', command: 'addColumnBefore' },
  { key: 'addColumnAfter', label: '向右插入一列', command: 'addColumnAfter' },
  { key: 'deleteColumn', label: '删除列', command: 'deleteColumn' },
];

const ROW_ACTIONS = [
  { key: 'toggleHeaderRow', label: '标题行', command: 'toggleHeaderRow' },
  { key: 'addRowBefore', label: '在上方插入行', command: 'addRowBefore' },
  { key: 'addRowAfter', label: '在下方插入行', command: 'addRowAfter' },
  { key: 'deleteRow', label: '删除行', command: 'deleteRow' },
];

const CELL_ACTIONS = [
  { key: 'mergeCells', label: '合并单元格', command: 'mergeCells' },
  { key: 'splitCell', label: '拆分单元格', command: 'splitCell' },
];

export default function RichTextTableToolbar({
  caption = '',
  editor,
  placement = 'above',
  position,
  visible,
}) {
  const rootRef = React.useRef(null);
  const [openMenu, setOpenMenu] = React.useState('');
  const [draftCaption, setDraftCaption] = React.useState(caption || '');

  React.useEffect(() => {
    if (!visible) setOpenMenu('');
  }, [visible]);

  React.useEffect(() => {
    if (!editor || !visible) return;
    const tableAttrs = editor.getAttributes('table') || {};
    setDraftCaption(tableAttrs.caption || caption || '');
  }, [caption, editor, visible]);

  React.useEffect(() => {
    if (!openMenu) return undefined;
    const handlePointerDown = (event) => {
      if (rootRef.current?.contains(event.target)) return;
      setOpenMenu('');
    };
    window.addEventListener('mousedown', handlePointerDown);
    return () => window.removeEventListener('mousedown', handlePointerDown);
  }, [openMenu]);

  if (!editor || !visible || !position) return null;

  const run = (commandName) => {
    const chain = editor.chain().focus();
    if (typeof chain[commandName] !== 'function') return;
    const didRun = chain[commandName]().run();
    if (didRun) setOpenMenu('');
  };

  const saveCaption = () => {
    const didRun = editor
      .chain()
      .focus()
      .updateAttributes('table', { caption: draftCaption })
      .run();
    if (didRun) setOpenMenu('');
  };

  const content = (
    <div
      className="rich-text-table-toolbar fixed z-[1120]"
      style={{
        left: position.left,
        top: position.top,
        transform: placement === 'below' ? 'translate(-50%, 0)' : 'translate(-50%, -100%)',
      }}
    >
      <div ref={rootRef} className="rich-text-table-toolbar-surface flex items-center gap-1 px-2 py-1.5">
        <div className="relative">
          <TableDropdown
            open={openMenu === 'column'}
            label="列操作"
            onToggle={() => setOpenMenu((current) => (current === 'column' ? '' : 'column'))}
          >
            <Columns3 size={15} />
          </TableDropdown>
          {openMenu === 'column' && (
            <div className="rich-text-table-menu absolute left-0 top-full mt-2 min-w-[180px]">
              {COLUMN_ACTIONS.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => run(item.command)}
                  className="rich-text-table-menu-item"
                >
                  {item.label}
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="relative">
          <TableDropdown
            open={openMenu === 'row'}
            label="行操作"
            onToggle={() => setOpenMenu((current) => (current === 'row' ? '' : 'row'))}
          >
            <Rows3 size={15} />
          </TableDropdown>
          {openMenu === 'row' && (
            <div className="rich-text-table-menu absolute left-0 top-full mt-2 min-w-[180px]">
              {ROW_ACTIONS.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => run(item.command)}
                  className="rich-text-table-menu-item"
                >
                  {item.label}
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="relative">
          <TableDropdown
            open={openMenu === 'cell'}
            label="单元格操作"
            onToggle={() => setOpenMenu((current) => (current === 'cell' ? '' : 'cell'))}
          >
            <Combine size={15} />
          </TableDropdown>
          {openMenu === 'cell' && (
            <div className="rich-text-table-menu absolute left-0 top-full mt-2 min-w-[180px]">
              {CELL_ACTIONS.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => run(item.command)}
                  className="rich-text-table-menu-item"
                >
                  {item.label}
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="relative">
          <TableToolbarButton
            active={openMenu === 'caption'}
            title="表格批注"
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => setOpenMenu((current) => (current === 'caption' ? '' : 'caption'))}
          >
            <Captions size={15} />
          </TableToolbarButton>
          {openMenu === 'caption' && (
            <div className="rich-text-table-menu absolute right-0 top-full mt-2 w-[320px] p-4">
              <div className="space-y-4">
                <div className="space-y-1">
                  <span className="block text-xs font-semibold uppercase tracking-[0.12em] text-gray-500 dark:text-gray-400">
                    表格批注
                  </span>
                  <Input
                    value={draftCaption}
                    onChange={(event) => setDraftCaption(event.target.value)}
                    placeholder="显示在表格下方的说明文字"
                  />
                </div>
                <div className="flex gap-2">
                  <Button size="sm" className="flex-1" onClick={saveCaption}>
                    保存
                  </Button>
                  <Button size="sm" variant="ghost" className="flex-1" onClick={() => setOpenMenu('')}>
                    取消
                  </Button>
                </div>
              </div>
            </div>
          )}
        </div>

        <TableToolbarButton
          title="删除表格"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => editor.chain().focus().deleteTable().run()}
        >
          <Trash2 size={15} />
        </TableToolbarButton>
      </div>
    </div>
  );

  if (typeof document === 'undefined') return content;
  return createPortal(content, document.body);
}
