import React from 'react';
import { EditorContent } from '@tiptap/react';
import RichTextBlockTrigger from './RichTextBlockTrigger';
import RichTextSelectionToolbar from './RichTextSelectionToolbar';
import RichTextTableToolbar from './RichTextTableToolbar';
import RichTextToolbar from './RichTextToolbar';

export default function RichTextEditorSurface({
  blockToolbarState,
  editor,
  imageInputRef,
  pendingTableConfig,
  pinnedSelections,
  selectionToolbarState,
  tableToolbarState,
  onApplyTableConfig,
  onBlockToolbarInteract,
  onBlockToolbarToggle,
  onHideBlockToolbar,
  onHideSelectionToolbar,
  onInsertImage,
  onPinSelection,
  onTableConfigChange,
}) {
  return (
    <div className="flex h-full min-h-0 flex-col bg-[#FAFAFA] text-gray-800 dark:bg-[#1E1E1E] dark:text-gray-200">
      <RichTextSelectionToolbar
        editor={editor}
        pinnedSelections={pinnedSelections}
        placement={selectionToolbarState.placement}
        position={selectionToolbarState.position}
        range={selectionToolbarState.range}
        visible={selectionToolbarState.visible && !tableToolbarState.visible}
        onPinSelection={onPinSelection}
        onRequestClose={onHideSelectionToolbar}
      />
      <RichTextTableToolbar
        caption={tableToolbarState.caption}
        editor={editor}
        placement={tableToolbarState.placement}
        position={tableToolbarState.position}
        visible={tableToolbarState.visible}
      />
      <RichTextToolbar
        editor={editor}
        floating
        pendingTableConfig={pendingTableConfig}
        position={blockToolbarState.toolbarPosition}
        visible={blockToolbarState.toolbarVisible && !tableToolbarState.visible}
        onApplyTableConfig={onApplyTableConfig}
        onInsertImage={() => imageInputRef.current?.click()}
        onInteractWithin={onBlockToolbarInteract}
        onRequestClose={onHideBlockToolbar}
        onTableConfigChange={onTableConfigChange}
      />
      <input
        ref={imageInputRef}
        type="file"
        accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml"
        className="hidden"
        onChange={onInsertImage}
      />
      <div className="relative min-h-0 flex-1 overflow-hidden">
        <RichTextBlockTrigger
          active={blockToolbarState.toolbarVisible}
          position={blockToolbarState.triggerPosition}
          visible={blockToolbarState.triggerVisible && !tableToolbarState.visible}
          onInteract={onBlockToolbarInteract}
          onToggle={onBlockToolbarToggle}
        />
        <EditorContent editor={editor} className="rich-text-editor-host h-full min-h-0" />
      </div>
    </div>
  );
}
