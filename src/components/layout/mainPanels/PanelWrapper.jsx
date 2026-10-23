import React from 'react';

export default function PanelWrapper({
  children,
  collectPanelSizes,
  hasOpenFloatingMenu,
  ide,
  index,
  nextPanel,
  panel,
  panelWidth,
  showResizer,
}) {
  const id = panel.id;

  return (
    <div
      key={id}
      id={`panel-wrapper-${id}`}
      className="panel-wrapper relative flex h-full min-w-0 overflow-visible"
      style={{
        width: `${panelWidth}px`,
        flex: 'none',
        opacity: ide.draggingPanelId === id ? 0.55 : 1,
        marginRight: showResizer ? '12px' : '0px',
        zIndex:
          hasOpenFloatingMenu
            ? 90
            : ide.activePanel === id || (ide.activePanel === 'chat-input' && id === 'chat')
            ? 40
            : 10 - index,
      }}
    >
      {ide.dragType === 'reorder' && ide.draggingPanelId !== id && (
        <div
          className={`ui-panel-surface absolute inset-0 z-[90] transition-colors ${
            ide.dragOverId === id ? 'ui-panel-surface-drop bg-blue-500/10' : ''
          }`}
          onDragOver={(e) => ide.handlePanelDragOver(e, id)}
          onDrop={(e) => ide.handlePanelDrop(e, id)}
          onDragLeave={() => ide.setDragOverId(null)}
        />
      )}

      <div className="h-full w-full min-w-0">
        {children}
      </div>

      {showResizer && (
        <div
          className="absolute -right-4 top-0 bottom-0 w-5 cursor-col-resize group flex items-center justify-center z-50"
          onMouseDown={(e) => ide.startResize(e, id, nextPanel.id, collectPanelSizes())}
        >
          <div className="h-10 w-1.5 rounded-full opacity-0 group-hover:opacity-100 transition-opacity duration-300 bg-gray-400/60 dark:bg-gray-500/60 group-active:bg-blue-500" />
        </div>
      )}
    </div>
  );
}
