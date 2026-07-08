import React from 'react';
import { createPortal } from 'react-dom';
import {
  Bold,
  CheckCheck,
  Heading2,
  Highlighter,
  Italic,
  List,
  ListOrdered,
  Palette,
  Quote,
  Strikethrough,
  Underline as UnderlineIcon,
} from 'lucide-react';
import { IconButton } from '../../ui';
import { FONT_SIZE_OPTIONS, getActiveFontSize } from './noteEditorExtensions';

const TEXT_COLOR_PRESETS = [
  '#111111',
  '#525252',
  '#737373',
  '#a3a3a3',
  '#dc2626',
  '#ea580c',
  '#ca8a04',
  '#16a34a',
  '#0891b2',
  '#2563eb',
  '#7c3aed',
  '#db2777',
];

const HIGHLIGHT_COLOR_PRESETS = [
  '#fef08a',
  '#fde68a',
  '#fecaca',
  '#bfdbfe',
  '#c7d2fe',
  '#ddd6fe',
  '#bbf7d0',
  '#a7f3d0',
  '#99f6e4',
  '#fbcfe8',
  '#fed7aa',
  '#e5e7eb',
];

function SelectionButton({ active = false, children, ...props }) {
  return (
    <IconButton size="xs" active={active} className="h-8 w-8 shrink-0" {...props}>
      {children}
    </IconButton>
  );
}

export default function RichTextSelectionToolbar({
  editor,
  pinnedSelections = [],
  placement = 'above',
  position,
  range,
  visible,
  onPinSelection,
  onRequestClose,
}) {
  const toolbarRef = React.useRef(null);
  const [isFontMenuOpen, setIsFontMenuOpen] = React.useState(false);
  const [isTextColorMenuOpen, setIsTextColorMenuOpen] = React.useState(false);
  const [isHighlightColorMenuOpen, setIsHighlightColorMenuOpen] = React.useState(false);

  React.useEffect(() => {
    setIsFontMenuOpen(false);
    setIsTextColorMenuOpen(false);
    setIsHighlightColorMenuOpen(false);
  }, [range?.from, range?.to, visible]);

  React.useEffect(() => {
    if (!visible && !isFontMenuOpen && !isTextColorMenuOpen && !isHighlightColorMenuOpen) {
      return undefined;
    }
    const handlePointerDown = (event) => {
      if (toolbarRef.current?.contains(event.target)) return;
      setIsFontMenuOpen(false);
      setIsTextColorMenuOpen(false);
      setIsHighlightColorMenuOpen(false);
      onRequestClose?.();
    };
    window.addEventListener('mousedown', handlePointerDown);
    return () => window.removeEventListener('mousedown', handlePointerDown);
  }, [isFontMenuOpen, isHighlightColorMenuOpen, isTextColorMenuOpen, onRequestClose, visible]);

  if (!editor || !visible || !position || !range) return null;

  const activeFontSize = getActiveFontSize(editor);
  const activeTextColor = editor.getAttributes('textStyle')?.color || '';
  const activeHighlightColor = editor.getAttributes('highlight')?.color || '';
  const isCurrentSelectionPinned =
    Array.isArray(pinnedSelections) &&
    pinnedSelections.some(
      (selection) => selection.start_offset === range.from && selection.end_offset === range.to
    );

  const applySelectionCommand = (runCommand) => {
    const chain = editor.chain().focus().setTextSelection({ from: range.from, to: range.to });
    runCommand(chain).run();
  };

  const content = (
    <div
      className="rich-text-selection-toolbar fixed"
      style={{
        left: position.left,
        top: position.top,
        transform: placement === 'below' ? 'translate(-50%, 0)' : 'translate(-50%, -100%)',
      }}
    >
      <div
        ref={toolbarRef}
        className="rich-text-selection-toolbar-surface flex items-center gap-1 px-2 py-1.5"
      >
        <SelectionButton
          active={isCurrentSelectionPinned}
          title="固定选区"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => onPinSelection?.()}
        >
          <CheckCheck size={15} />
        </SelectionButton>

        <div className="relative">
          <button
            type="button"
            title="字号"
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => {
              setIsTextColorMenuOpen(false);
              setIsHighlightColorMenuOpen(false);
              setIsFontMenuOpen((current) => !current);
            }}
            className={`motion-button rich-text-font-trigger inline-flex h-8 w-8 items-center justify-center rounded-xl text-sm font-semibold ${
              isFontMenuOpen ? 'rich-text-font-trigger-active' : ''
            }`}
          >
            <span className="text-[15px] font-bold leading-none">A</span>
          </button>
          {isFontMenuOpen && (
            <div className="rich-text-submenu-panel rich-text-submenu-panel-surface absolute left-0 top-full mt-2 min-w-[98px] p-1">
              {FONT_SIZE_OPTIONS.map((value) => (
                <button
                  key={value}
                  type="button"
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => {
                    applySelectionCommand((chain) => chain.setFontSize(value));
                    setIsFontMenuOpen(false);
                  }}
                  className={`motion-row rich-text-submenu-item block w-full rounded-xl px-3 py-2 text-left text-sm font-semibold ${
                    value === activeFontSize ? 'rich-text-submenu-item-active' : ''
                  }`}
                >
                  {value}px
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="relative">
          <button
            type="button"
            title="字体颜色"
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => {
              setIsFontMenuOpen(false);
              setIsHighlightColorMenuOpen(false);
              setIsTextColorMenuOpen((current) => !current);
            }}
            className={`motion-button rich-text-font-trigger inline-flex h-8 w-8 items-center justify-center rounded-xl ${
              isTextColorMenuOpen || !!activeTextColor ? 'rich-text-font-trigger-active' : ''
            }`}
          >
            <span className="rich-text-color-trigger-glyph">
              <Palette size={14} />
              <span
                className="rich-text-color-trigger-bar"
                style={{ backgroundColor: activeTextColor || '#3b82f6' }}
              />
            </span>
          </button>
          {isTextColorMenuOpen && (
            <div className="rich-text-color-menu rich-text-submenu-panel rich-text-submenu-panel-surface absolute left-0 top-full mt-2 w-[188px] p-2">
              <button
                type="button"
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => {
                  applySelectionCommand((chain) => chain.unsetColor());
                  setIsTextColorMenuOpen(false);
                }}
                className="motion-row rich-text-color-menu-action"
              >
                去除颜色
              </button>
              <div className="rich-text-color-grid">
                {TEXT_COLOR_PRESETS.map((color) => (
                  <button
                    key={color}
                    type="button"
                    aria-label={`选择 ${color} 文字颜色`}
                    onMouseDown={(event) => event.preventDefault()}
                    onClick={() => {
                      applySelectionCommand((chain) => chain.setColor(color));
                      setIsTextColorMenuOpen(false);
                    }}
                    className={`rich-text-color-swatch ${activeTextColor === color ? 'is-active' : ''}`}
                    style={{ backgroundColor: color }}
                  />
                ))}
              </div>
              <label className="rich-text-color-menu-custom">
                <span>颜色选择器</span>
                <input
                  type="color"
                  value={activeTextColor || '#3b82f6'}
                  onMouseDown={(event) => event.preventDefault()}
                  onChange={(event) =>
                    applySelectionCommand((chain) => chain.setColor(event.target.value))
                  }
                />
              </label>
            </div>
          )}
        </div>

        <div className="relative">
          <button
            type="button"
            title="字体背景色"
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => {
              setIsFontMenuOpen(false);
              setIsTextColorMenuOpen(false);
              setIsHighlightColorMenuOpen((current) => !current);
            }}
            className={`motion-button rich-text-font-trigger inline-flex h-8 w-8 items-center justify-center rounded-xl ${
              isHighlightColorMenuOpen || editor.isActive('highlight')
                ? 'rich-text-font-trigger-active'
                : ''
            }`}
          >
            <span className="rich-text-highlight-trigger-glyph">
              <span
                className="rich-text-highlight-trigger-fill"
                style={{ backgroundColor: activeHighlightColor || '#fef08a' }}
              />
              <span className="rich-text-highlight-trigger-letter">A</span>
            </span>
          </button>
          {isHighlightColorMenuOpen && (
            <div className="rich-text-color-menu rich-text-submenu-panel rich-text-submenu-panel-surface absolute left-0 top-full mt-2 w-[188px] p-2">
              <button
                type="button"
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => {
                  applySelectionCommand((chain) => chain.unsetHighlight());
                  setIsHighlightColorMenuOpen(false);
                }}
                className="motion-row rich-text-color-menu-action"
              >
                去除背景色
              </button>
              <div className="rich-text-color-grid">
                {HIGHLIGHT_COLOR_PRESETS.map((color) => (
                  <button
                    key={color}
                    type="button"
                    aria-label={`选择 ${color} 背景色`}
                    onMouseDown={(event) => event.preventDefault()}
                    onClick={() => {
                      applySelectionCommand((chain) => chain.setHighlight({ color }));
                      setIsHighlightColorMenuOpen(false);
                    }}
                    className={`rich-text-color-swatch ${activeHighlightColor === color ? 'is-active' : ''}`}
                    style={{ backgroundColor: color }}
                  />
                ))}
              </div>
              <label className="rich-text-color-menu-custom">
                <span>颜色选择器</span>
                <input
                  type="color"
                  value={activeHighlightColor || '#fef08a'}
                  onMouseDown={(event) => event.preventDefault()}
                  onChange={(event) =>
                    applySelectionCommand((chain) =>
                      chain.setHighlight({ color: event.target.value })
                    )
                  }
                />
              </label>
            </div>
          )}
        </div>

        <SelectionButton
          active={editor.isActive('bold')}
          title="加粗"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => applySelectionCommand((chain) => chain.toggleBold())}
        >
          <Bold size={15} />
        </SelectionButton>
        <SelectionButton
          active={editor.isActive('italic')}
          title="斜体"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => applySelectionCommand((chain) => chain.toggleItalic())}
        >
          <Italic size={15} />
        </SelectionButton>
        <SelectionButton
          active={editor.isActive('underline')}
          title="下划线"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => applySelectionCommand((chain) => chain.toggleUnderline())}
        >
          <UnderlineIcon size={15} />
        </SelectionButton>
        <SelectionButton
          active={editor.isActive('strike')}
          title="删除线"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => applySelectionCommand((chain) => chain.toggleStrike())}
        >
          <Strikethrough size={15} />
        </SelectionButton>
        <SelectionButton
          active={editor.isActive('highlight')}
          title="高亮"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() =>
            applySelectionCommand((chain) =>
              editor.isActive('highlight')
                ? chain.unsetHighlight()
                : chain.setHighlight({ color: '#fef08a' })
            )
          }
        >
          <Highlighter size={15} />
        </SelectionButton>
        <SelectionButton
          active={editor.isActive('heading', { level: 2 })}
          title="二级标题"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => applySelectionCommand((chain) => chain.toggleHeading({ level: 2 }))}
        >
          <Heading2 size={15} />
        </SelectionButton>
        <SelectionButton
          active={editor.isActive('bulletList')}
          title="无序列表"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => applySelectionCommand((chain) => chain.toggleBulletList())}
        >
          <List size={15} />
        </SelectionButton>
        <SelectionButton
          active={editor.isActive('orderedList')}
          title="有序列表"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => applySelectionCommand((chain) => chain.toggleOrderedList())}
        >
          <ListOrdered size={15} />
        </SelectionButton>
        <SelectionButton
          active={editor.isActive('blockquote')}
          title="引用"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => applySelectionCommand((chain) => chain.toggleBlockquote())}
        >
          <Quote size={15} />
        </SelectionButton>
      </div>
    </div>
  );

  if (typeof document === 'undefined') return content;
  return createPortal(content, document.body);
}
