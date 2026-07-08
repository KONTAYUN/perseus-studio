import React from 'react';
import { createPortal } from 'react-dom';
import {
  AlignCenter,
  AlignLeft,
  AlignRight,
  ChevronRight,
  ImagePlus,
  SlidersHorizontal,
  Table2,
} from 'lucide-react';
import { Checkbox, IconButton } from '../../ui';
import {
  getActiveLineHeight,
  getActiveParagraphSpacing,
  LINE_HEIGHT_OPTIONS,
  PARAGRAPH_SPACING_OPTIONS,
} from './noteEditorExtensions';

function ToolbarButton({ children, ...props }) {
  return (
    <IconButton size="xs" className="h-8 w-8 shrink-0" {...props}>
      {children}
    </IconButton>
  );
}

const SPACING_GROUPS = [
  {
    key: 'lineHeight',
    label: '行距',
    options: LINE_HEIGHT_OPTIONS.map((item) => ({ value: item, label: String(item) })),
  },
  {
    key: 'paragraphSpacing',
    label: '段距',
    options: PARAGRAPH_SPACING_OPTIONS.map((item) => ({ value: item, label: `${item}em` })),
  },
];

const TABLE_PICKER_ROWS = 10;
const TABLE_PICKER_COLS = 10;

export default function RichTextToolbar({
  editor,
  floating = false,
  onApplyTableConfig,
  onInsertImage,
  onInteractWithin,
  onRequestClose,
  onTableConfigChange,
  pendingTableConfig,
  position = null,
  visible = true,
}) {
  const toolbarRef = React.useRef(null);
  const [isSpacingMenuOpen, setIsSpacingMenuOpen] = React.useState(false);
  const [isTableMenuOpen, setIsTableMenuOpen] = React.useState(false);
  const [activeSpacingMenu, setActiveSpacingMenu] = React.useState('');
  const [hoveredTableSize, setHoveredTableSize] = React.useState({ rows: 0, cols: 0 });

  React.useEffect(() => {
    if (!isSpacingMenuOpen) {
      setActiveSpacingMenu('');
    }
  }, [isSpacingMenuOpen]);

  React.useEffect(() => {
    if (!isTableMenuOpen) return;
    setHoveredTableSize({
      rows: Math.min(TABLE_PICKER_ROWS, Math.max(1, Number(pendingTableConfig?.rows) || 3)),
      cols: Math.min(TABLE_PICKER_COLS, Math.max(1, Number(pendingTableConfig?.cols) || 3)),
    });
  }, [isTableMenuOpen, pendingTableConfig?.cols, pendingTableConfig?.rows]);

  React.useEffect(() => {
    if ((!isSpacingMenuOpen && !isTableMenuOpen) && !(floating && visible)) return undefined;
    const handlePointerDown = (event) => {
      if (toolbarRef.current?.contains(event.target)) return;
      setIsSpacingMenuOpen(false);
      setIsTableMenuOpen(false);
      onRequestClose?.();
    };
    window.addEventListener('mousedown', handlePointerDown);
    return () => window.removeEventListener('mousedown', handlePointerDown);
  }, [floating, isSpacingMenuOpen, isTableMenuOpen, onRequestClose, visible]);

  React.useEffect(() => {
    if (!visible) {
      setIsSpacingMenuOpen(false);
      setIsTableMenuOpen(false);
      setActiveSpacingMenu('');
    }
  }, [visible]);

  if (!editor || !visible) return null;

  const spacingGroups = SPACING_GROUPS.map((group) => ({
    ...group,
    value:
      group.key === 'lineHeight'
        ? String(getActiveLineHeight(editor))
        : `${getActiveParagraphSpacing(editor)}em`,
    apply:
      group.key === 'lineHeight'
        ? (value) => editor.chain().focus().setLineHeight(value).run()
        : (value) => editor.chain().focus().setParagraphSpacing(value).run(),
  }));
  const activeSpacingGroup = spacingGroups.find((item) => item.key === activeSpacingMenu) || null;

  const content = (
    <div
      ref={toolbarRef}
      onMouseDownCapture={() => onInteractWithin?.()}
      className={
        floating
          ? 'rich-text-toolbar-floating fixed'
          : 'rich-text-toolbar relative flex flex-wrap items-center gap-2 border-b border-gray-200/70 bg-white/92 px-4 py-2 dark:border-white/10 dark:bg-[#1f2023]'
      }
      style={
        floating && position
          ? {
              left: position.left,
              top: position.top,
            }
          : undefined
      }
    >
      {floating && <span className="rich-text-toolbar-floating-arrow" aria-hidden="true" />}
      <div
        className={
          floating
            ? 'rich-text-toolbar-floating-surface flex flex-wrap items-center gap-2 px-3 py-2'
            : 'flex items-center gap-2'
        }
      >
        <div className="flex items-center gap-1">
          <ToolbarButton
            active={editor.isActive({ textAlign: 'left' })}
            title="左对齐"
            onClick={() => editor.chain().focus().setTextAlign('left').run()}
          >
            <AlignLeft size={15} />
          </ToolbarButton>
          <ToolbarButton
            active={editor.isActive({ textAlign: 'center' })}
            title="居中"
            onClick={() => editor.chain().focus().setTextAlign('center').run()}
          >
            <AlignCenter size={15} />
          </ToolbarButton>
          <ToolbarButton
            active={editor.isActive({ textAlign: 'right' })}
            title="右对齐"
            onClick={() => editor.chain().focus().setTextAlign('right').run()}
          >
            <AlignRight size={15} />
          </ToolbarButton>
        </div>

        <div className="h-6 w-px bg-gray-200 dark:bg-white/10" />

        <div className="relative">
          <button
            type="button"
            title="段落样式"
            onClick={() => {
              setIsTableMenuOpen(false);
              setIsSpacingMenuOpen((current) => !current);
            }}
            className={`motion-button rich-text-toolbar-trigger inline-flex h-8 w-8 items-center justify-center ${
              isSpacingMenuOpen ? 'rich-text-toolbar-trigger-active' : ''
            }`}
          >
            <SlidersHorizontal size={15} />
          </button>
          {isSpacingMenuOpen && (
            <div className="absolute left-0 top-full mt-2 flex items-start">
              <div className="rich-text-submenu-panel rich-text-submenu-panel-surface flex w-[108px] flex-col gap-1 p-1">
                {spacingGroups.map((group) => (
                  <button
                    key={group.key}
                    type="button"
                    onClick={() =>
                      setActiveSpacingMenu((current) => (current === group.key ? '' : group.key))
                    }
                    className={`motion-row rich-text-submenu-item flex items-center justify-between rounded-xl px-3 py-2 text-sm font-semibold ${
                      activeSpacingMenu === group.key ? 'rich-text-submenu-item-active' : ''
                    }`}
                  >
                    <span>{group.label}</span>
                    <ChevronRight size={12} />
                  </button>
                ))}
              </div>

              {activeSpacingGroup && (
                <div className="rich-text-submenu-panel rich-text-submenu-panel-surface ml-2 flex w-[108px] flex-col gap-1 p-1">
                  {activeSpacingGroup.options.map((option) => (
                    <button
                      key={`${activeSpacingGroup.key}-${option.value}`}
                      type="button"
                      onMouseDown={(event) => event.preventDefault()}
                      onClick={() => {
                        activeSpacingGroup.apply(option.value);
                        setIsSpacingMenuOpen(false);
                      }}
                      className={`motion-row rich-text-submenu-item rounded-xl px-3 py-2 text-left text-sm font-semibold ${
                        option.label === activeSpacingGroup.value
                          ? 'rich-text-submenu-item-active'
                          : ''
                      }`}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        <div className="h-6 w-px bg-gray-200 dark:bg-white/10" />

        <div className="relative">
          <ToolbarButton
            active={isTableMenuOpen}
            title="插入表格"
            onClick={() => {
              setIsSpacingMenuOpen(false);
              setIsTableMenuOpen((current) => !current);
            }}
          >
            <Table2 size={15} />
          </ToolbarButton>
          {isTableMenuOpen && (
            <div className="rich-text-submenu-panel rich-text-submenu-panel-surface absolute left-0 top-full mt-2 w-[244px] p-3">
              <div className="space-y-3">
                <div
                  className="table-size-picker"
                  onMouseLeave={() =>
                    setHoveredTableSize({
                      rows: Math.min(
                        TABLE_PICKER_ROWS,
                        Math.max(1, Number(pendingTableConfig?.rows) || 3)
                      ),
                      cols: Math.min(
                        TABLE_PICKER_COLS,
                        Math.max(1, Number(pendingTableConfig?.cols) || 3)
                      ),
                    })
                  }
                >
                  <div className="table-size-picker-grid">
                    {Array.from({ length: TABLE_PICKER_ROWS * TABLE_PICKER_COLS }, (_, index) => {
                      const row = Math.floor(index / TABLE_PICKER_COLS) + 1;
                      const col = (index % TABLE_PICKER_COLS) + 1;
                      const active = row <= hoveredTableSize.rows && col <= hoveredTableSize.cols;
                      return (
                        <button
                          key={`table-cell-${row}-${col}`}
                          type="button"
                          className={`table-size-picker-cell ${active ? 'is-active' : ''}`}
                          aria-label={`插入 ${row} 行 ${col} 列表格`}
                          onMouseEnter={() => setHoveredTableSize({ rows: row, cols: col })}
                          onFocus={() => setHoveredTableSize({ rows: row, cols: col })}
                          onClick={() => {
                            onTableConfigChange?.((current) => ({
                              ...current,
                              rows: String(row),
                              cols: String(col),
                            }));
                            onApplyTableConfig?.({
                              ...pendingTableConfig,
                              rows: String(row),
                              cols: String(col),
                            });
                            setIsTableMenuOpen(false);
                          }}
                        />
                      );
                    })}
                  </div>
                  <div className="table-size-picker-label">
                    {hoveredTableSize.rows} x {hoveredTableSize.cols}
                  </div>
                </div>
                <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                  <Checkbox
                    checked={pendingTableConfig?.withHeaderRow ?? true}
                    onChange={(checked) =>
                      onTableConfigChange?.((current) => ({
                        ...current,
                        withHeaderRow: checked,
                      }))
                    }
                  />
                  <span>首行作为表头</span>
                </label>
              </div>
            </div>
          )}
        </div>

        <ToolbarButton title="插入图片" onClick={onInsertImage}>
          <ImagePlus size={15} />
        </ToolbarButton>
      </div>
    </div>
  );

  if (!floating || typeof document === 'undefined') return content;
  return createPortal(content, document.body);
}
