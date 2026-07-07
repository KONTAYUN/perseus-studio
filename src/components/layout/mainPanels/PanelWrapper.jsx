import React from 'react';

export default function PanelWrapper({
  children,
  collectPanelSizes,
  hasOpenFloatingMenu,
  ide,
  index,
  isOpeningPanel,
  isResizing,
  nextPanel,
  panel,
  panelWidth,
  shouldIntro,
  showResizer,
}) {
  const id = panel.id;

  return (
    <div
      key={id}
      id={`panel-wrapper-${id}`}
      className={`panel-wrapper relative flex h-full min-w-0 will-change-[transform,opacity] ${
        isOpeningPanel || shouldIntro ? 'overflow-hidden' : 'overflow-visible'
      } ${
        isResizing ? '' : 'motion-layout'
      }`}
      style={{
        width: `${isOpeningPanel ? 0 : panelWidth}px`,
        flex: 'none',
        opacity: isOpeningPanel ? 0 : ide.draggingPanelId === id ? 0.55 : 1,
        transform: ide.draggingPanelId === id
          ? 'scale(0.985) translateY(2px)'
          : 'scale(1) translateY(0)',
        marginRight: showResizer && !isOpeningPanel ? '12px' : '0px',
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
          className={`motion-color absolute inset-0 z-[90] rounded-3xl border ${
            ide.dragOverId === id
              ? 'border-blue-400/60 bg-blue-500/10'
              : 'border-transparent bg-transparent'
          }`}
          onDragOver={(e) => ide.handlePanelDragOver(e, id)}
          onDrop={(e) => ide.handlePanelDrop(e, id)}
          onDragLeave={() => ide.setDragOverId(null)}
        />
      )}

      <div
        className={`h-full w-full min-w-0 ${
          shouldIntro ? 'motion-panel-enter' : ''
        }`}
        style={shouldIntro ? { animationDelay: `${Math.min(80, index * 24)}ms` } : undefined}
      >
        {children}
      </div>

      {showResizer && (
        <div
          className="absolute -right-4 top-0 bottom-0 w-5 cursor-col-resize group flex items-center justify-center z-50"
          onMouseDown={(e) => ide.startResize(e, id, nextPanel.id, collectPanelSizes())}
        >
          <div className="h-10 w-1.5 rounded-full opacity-0 group-hover:opacity-100 motion-row bg-gray-400/60 dark:bg-gray-500/60 group-active:bg-blue-500" />
        </div>
      )}
    </div>
  );
}
