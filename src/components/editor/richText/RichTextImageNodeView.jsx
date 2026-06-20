import React from 'react';
import { NodeViewWrapper } from '@tiptap/react';
import {
  AlignCenter,
  AlignLeft,
  AlignRight,
  Maximize2,
  Settings2,
} from 'lucide-react';
import { Button, IconButton, Input } from '../../ui';

const IMAGE_SIZE_PRESETS = ['33', '50', '66', '100'];
const BORDER_STYLE_OPTIONS = [
  { value: 'none', label: '无' },
  { value: 'solid', label: '实线' },
  { value: 'dashed', label: '虚线' },
];

function clampWidth(value) {
  return Math.min(100, Math.max(20, Number(value) || 100));
}

function TableToolbarButton({ active = false, children, ...props }) {
  return (
    <IconButton size="xs" active={active} className="h-8 w-8 shrink-0" {...props}>
      {children}
    </IconButton>
  );
}

export default function RichTextImageNodeView({
  editor,
  getPos,
  node,
  selected,
  updateAttributes,
}) {
  const wrapperRef = React.useRef(null);
  const dragStateRef = React.useRef(null);
  const detachDragListenersRef = React.useRef(null);
  const [isPanelOpen, setIsPanelOpen] = React.useState(false);
  const [draft, setDraft] = React.useState({
    align: 'center',
    backgroundColor: 'transparent',
    borderColor: '#cbd5e1',
    borderRadius: '18',
    borderStyle: 'none',
    borderWidth: '0',
    widthPercent: '100',
  });

  const align = node.attrs.align || 'center';
  const widthPercent = clampWidth(node.attrs.widthPercent || 100);
  const alt = node.attrs.alt || '';

  React.useEffect(() => {
    setDraft({
      align,
      backgroundColor: node.attrs.backgroundColor || 'transparent',
      borderColor: node.attrs.borderColor || '#cbd5e1',
      borderRadius: node.attrs.borderRadius || '18',
      borderStyle: node.attrs.borderStyle || 'none',
      borderWidth: node.attrs.borderWidth || '0',
      widthPercent: String(widthPercent),
    });
  }, [
    align,
    node.attrs.backgroundColor,
    node.attrs.borderColor,
    node.attrs.borderRadius,
    node.attrs.borderStyle,
    node.attrs.borderWidth,
    widthPercent,
  ]);

  React.useEffect(
    () => () => {
      detachDragListenersRef.current?.();
    },
    []
  );

  const beginResize = (event, direction) => {
    event.preventDefault();
    event.stopPropagation();
    const wrapper = wrapperRef.current;
    const container = wrapper?.parentElement;
    if (!wrapper || !container) return;
    editor.chain().focus().setNodeSelection(getPos()).run();
    dragStateRef.current = {
      containerWidth: container.getBoundingClientRect().width,
      direction,
      startWidth: widthPercent,
      startX: event.clientX,
    };
    const handlePointerMove = (moveEvent) => {
      if (!dragStateRef.current) return;
      const {
        startWidth,
        startX,
        containerWidth: currentContainerWidth,
        direction: currentDirection,
      } = dragStateRef.current;
      const delta = moveEvent.clientX - startX;
      const widthDelta = (delta / Math.max(currentContainerWidth, 1)) * 100 * currentDirection;
      const nextWidth = String(clampWidth(startWidth + widthDelta));
      updateAttributes({ widthPercent: nextWidth });
      setDraft((current) => ({ ...current, widthPercent: nextWidth }));
    };
    const handlePointerUp = () => {
      dragStateRef.current = null;
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', handlePointerUp);
      detachDragListenersRef.current = null;
    };
    detachDragListenersRef.current?.();
    detachDragListenersRef.current = () => {
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', handlePointerUp);
    };
    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', handlePointerUp, { once: true });
  };

  const setAlignment = (nextAlign) => {
    editor.chain().focus().setNodeSelection(getPos()).run();
    updateAttributes({ align: nextAlign });
    setDraft((current) => ({ ...current, align: nextAlign }));
  };

  const setPresetWidth = (nextWidth) => {
    editor.chain().focus().setNodeSelection(getPos()).run();
    updateAttributes({ widthPercent: String(nextWidth) });
    setDraft((current) => ({ ...current, widthPercent: String(nextWidth) }));
  };

  const alignmentClass =
    align === 'left' ? 'mr-auto ml-0' : align === 'right' ? 'ml-auto mr-0' : 'mx-auto';

  return (
    <NodeViewWrapper
      className={`note-image-node my-6 ${selected ? 'note-image-node-selected' : ''}`}
      data-align={align}
      data-width={widthPercent}
      contentEditable={false}
      onClick={() => {
        editor.chain().focus().setNodeSelection(getPos()).run();
      }}
    >
      {selected && (
        <div className="note-image-toolbar">
          <div className="note-image-toolbar-group">
            <button
              type="button"
              className={`note-image-toolbar-button ${align === 'left' ? 'is-active' : ''}`}
              onClick={() => setAlignment('left')}
              title="左对齐"
            >
              <AlignLeft size={14} />
            </button>
            <button
              type="button"
              className={`note-image-toolbar-button ${align === 'center' ? 'is-active' : ''}`}
              onClick={() => setAlignment('center')}
              title="居中"
            >
              <AlignCenter size={14} />
            </button>
            <button
              type="button"
              className={`note-image-toolbar-button ${align === 'right' ? 'is-active' : ''}`}
              onClick={() => setAlignment('right')}
              title="右对齐"
            >
              <AlignRight size={14} />
            </button>
          </div>
          <div className="note-image-toolbar-group">
            {IMAGE_SIZE_PRESETS.map((preset) => (
              <button
                key={preset}
                type="button"
                className={`note-image-toolbar-button ${String(widthPercent) === preset ? 'is-active' : ''}`}
                onClick={() => setPresetWidth(preset)}
                title={`设置为 ${preset}%`}
              >
                {preset}%
              </button>
            ))}
            <button
              type="button"
              className="note-image-toolbar-button"
              onClick={() => setPresetWidth(100)}
              title="重置宽度"
            >
              <Maximize2 size={14} />
            </button>
          </div>
          <div className="note-image-toolbar-group">
            <button
              type="button"
              className={`note-image-toolbar-button ${isPanelOpen ? 'is-active' : ''}`}
              onClick={() => setIsPanelOpen((current) => !current)}
              title="图片属性"
            >
              <Settings2 size={14} />
            </button>
          </div>
        </div>
      )}
      {selected && isPanelOpen && (
        <div className="note-image-property-panel">
          <div className="space-y-4">
            <div className="text-sm font-semibold text-gray-900 dark:text-white">图片属性</div>
            <div className="grid grid-cols-2 gap-3">
              <label className="space-y-1">
                <span className="block text-xs font-semibold text-gray-500 dark:text-gray-400">宽度</span>
                <Input
                  value={draft.widthPercent}
                  onChange={(event) =>
                    setDraft((current) => ({ ...current, widthPercent: event.target.value }))
                  }
                  placeholder="100"
                />
              </label>
              <label className="space-y-1">
                <span className="block text-xs font-semibold text-gray-500 dark:text-gray-400">圆角</span>
                <Input
                  value={draft.borderRadius}
                  onChange={(event) =>
                    setDraft((current) => ({ ...current, borderRadius: event.target.value }))
                  }
                  placeholder="18"
                />
              </label>
              <label className="space-y-1">
                <span className="block text-xs font-semibold text-gray-500 dark:text-gray-400">边框样式</span>
                <select
                  value={draft.borderStyle}
                  onChange={(event) =>
                    setDraft((current) => ({ ...current, borderStyle: event.target.value }))
                  }
                  className="h-10 w-full rounded-xl border border-gray-200 bg-white px-3 text-sm dark:border-white/10 dark:bg-[#1f2023]"
                >
                  {BORDER_STYLE_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>
              <label className="space-y-1">
                <span className="block text-xs font-semibold text-gray-500 dark:text-gray-400">边框宽度</span>
                <Input
                  value={draft.borderWidth}
                  onChange={(event) =>
                    setDraft((current) => ({ ...current, borderWidth: event.target.value }))
                  }
                  placeholder="0"
                />
              </label>
              <label className="space-y-1">
                <span className="block text-xs font-semibold text-gray-500 dark:text-gray-400">边框颜色</span>
                <Input
                  type="color"
                  value={draft.borderColor}
                  onChange={(event) =>
                    setDraft((current) => ({ ...current, borderColor: event.target.value }))
                  }
                  className="h-10"
                />
              </label>
              <label className="space-y-1">
                <span className="block text-xs font-semibold text-gray-500 dark:text-gray-400">背景</span>
                <Input
                  type="color"
                  value={draft.backgroundColor === 'transparent' ? '#ffffff' : draft.backgroundColor}
                  onChange={(event) =>
                    setDraft((current) => ({ ...current, backgroundColor: event.target.value }))
                  }
                  className="h-10"
                />
              </label>
            </div>
            <div className="space-y-1">
              <span className="block text-xs font-semibold text-gray-500 dark:text-gray-400">对齐</span>
              <div className="flex items-center gap-1 rounded-xl border border-gray-200 p-1 dark:border-white/10">
                <TableToolbarButton
                  active={draft.align === 'left'}
                  onClick={() => setDraft((current) => ({ ...current, align: 'left' }))}
                >
                  <AlignLeft size={14} />
                </TableToolbarButton>
                <TableToolbarButton
                  active={draft.align === 'center'}
                  onClick={() => setDraft((current) => ({ ...current, align: 'center' }))}
                >
                  <AlignCenter size={14} />
                </TableToolbarButton>
                <TableToolbarButton
                  active={draft.align === 'right'}
                  onClick={() => setDraft((current) => ({ ...current, align: 'right' }))}
                >
                  <AlignRight size={14} />
                </TableToolbarButton>
              </div>
            </div>
            <div className="flex gap-2">
              <Button
                size="sm"
                className="flex-1"
                onClick={() => {
                  updateAttributes({
                    align: draft.align,
                    backgroundColor: draft.backgroundColor,
                    borderColor: draft.borderColor,
                    borderRadius: draft.borderRadius,
                    borderStyle: draft.borderStyle,
                    borderWidth: draft.borderWidth,
                    widthPercent: String(clampWidth(draft.widthPercent)),
                  });
                  setIsPanelOpen(false);
                }}
              >
                保存
              </Button>
              <Button size="sm" variant="ghost" className="flex-1" onClick={() => setIsPanelOpen(false)}>
                取消
              </Button>
            </div>
          </div>
        </div>
      )}
      <div ref={wrapperRef} className={`note-image-frame ${alignmentClass}`} style={{ width: `${widthPercent}%` }}>
        <img
          src={node.attrs.src}
          alt={alt}
          title={node.attrs.title || alt}
          data-note-src={node.attrs['data-note-src'] || ''}
          data-align={align}
          data-width={widthPercent}
          draggable="false"
        />
        {selected && (
          <>
            <button
              type="button"
              className="note-image-resize-handle note-image-resize-handle-left"
              onPointerDown={(event) => beginResize(event, -1)}
              title="拖拽调整图片宽度"
            />
            <button
              type="button"
              className="note-image-resize-handle note-image-resize-handle-right"
              onPointerDown={(event) => beginResize(event, 1)}
              title="拖拽调整图片宽度"
            />
          </>
        )}
      </div>
    </NodeViewWrapper>
  );
}
