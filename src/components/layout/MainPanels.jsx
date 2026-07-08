import React, { useMemo, useRef } from 'react';
import ChatPanel from '../chat/ChatPanel';
import EditorPanel from '../editor/EditorPanel';
import SidebarPanel from '../sidebar/SidebarPanel';
import PanelWrapper from './mainPanels/PanelWrapper';
import PanelExitGhost from './mainPanels/PanelExitGhost';
import {
  calculateDisplayWidths,
  collectPanelSizes,
  defaultWidthForPanel,
  isCenteredSingleChatPanel,
  MAIN_HORIZONTAL_PADDING,
  PANEL_GAP,
} from './mainPanels/panelWidthUtils';
import { useMeasuredMainWidth } from './mainPanels/useMeasuredMainWidth';
import { usePanelActiveGlow } from './mainPanels/usePanelActiveGlow.jsx';
import { usePanelMotion } from './mainPanels/usePanelMotion';

function getPanelContent(panel, ide) {
  if (panel.type === 'fileTree') return <SidebarPanel ide={ide} panelId={panel.id} />;
  if (panel.type === 'editor') {
    const editor = ide.getEditorPanelState(panel.id);
    return <EditorPanel ide={ide} editor={editor} panelId={panel.id} />;
  }
  if (panel.type === 'chat') return <ChatPanel ide={ide} panelId={panel.id} />;
  return null;
}

export default function MainPanels({ ide }) {
  const mainRef = useRef(null);
  const visiblePanels = ide.panels;
  const mainWidth = useMeasuredMainWidth(mainRef);
  const centerSinglePanel = isCenteredSingleChatPanel(visiblePanels);
  const contentWidth = Math.max(0, mainWidth - MAIN_HORIZONTAL_PADDING);
  const isResizing = ide.dragType === 'resize';
  const displayWidths = useMemo(
    () => calculateDisplayWidths({
      mainWidth,
      panels: visiblePanels,
      widths: ide.widths,
      panelGap: PANEL_GAP,
    }),
    [ide.widths, mainWidth, visiblePanels]
  );
  const activeGlowPortal = usePanelActiveGlow({
    activePanel: ide.activePanel,
    displayWidths,
    dragType: ide.dragType,
    draggingPanelId: ide.draggingPanelId,
    isDraggingMode: ide.isDraggingMode,
    layoutAnimationAction: ide.layoutAnimationAction,
    mainWidth,
    visiblePanels,
  });
  const { getPanelIntroState } = usePanelMotion({
    displayWidths,
    draggingPanelId: ide.draggingPanelId,
    isDraggingMode: ide.isDraggingMode,
    isResizing,
    layoutAnimationAction: ide.layoutAnimationAction,
    mainWidth,
    setLayoutAnimationAction: ide.setLayoutAnimationAction,
    visiblePanels,
  });
  const collectVisiblePanelSizes = () => collectPanelSizes(visiblePanels, displayWidths);

  return (
    <>
      {activeGlowPortal}
      {ide.exitingPanelGhosts?.map((ghost) => (
        <PanelExitGhost key={ghost.key} ghost={ghost} />
      ))}
      <main
        ref={mainRef}
        className="flex-1 flex overflow-hidden p-3 relative z-10"
        onClick={(e) => e.target.tagName === 'MAIN' && ide.setActivePanel('none')}
      >
        {ide.isDraggingMode && (
          <div
            className={`fixed inset-0 z-[100] pointer-events-none ${
              ide.dragType === 'resize' ? 'cursor-col-resize' : 'cursor-grabbing'
            }`}
          />
        )}

        {centerSinglePanel && <div className="flex-1" />}

        {visiblePanels.map((panel, index) => {
          const id = panel.id;
          const nextPanel = visiblePanels[index + 1] || null;
          const showResizer = Boolean(nextPanel);
          const panelWidth =
            displayWidths[id] ||
            defaultWidthForPanel(panel, { centerSinglePanel, contentWidth });
          const hasOpenFloatingMenu =
            (panel.type === 'chat' && (ide.isModelMenuOpen || ide.isConversationMenuOpen)) ||
            (panel.type === 'fileTree' && ide.isAddMenuOpen);
          const { isOpeningPanel, shouldIntro } = getPanelIntroState(id);

          return (
            <PanelWrapper
              key={id}
              collectPanelSizes={collectVisiblePanelSizes}
              hasOpenFloatingMenu={hasOpenFloatingMenu}
              ide={ide}
              index={index}
              isOpeningPanel={isOpeningPanel}
              isResizing={isResizing}
              nextPanel={nextPanel}
              panel={panel}
              panelWidth={panelWidth}
              shouldIntro={shouldIntro}
              showResizer={showResizer}
            >
              {getPanelContent(panel, ide)}
            </PanelWrapper>
          );
        })}

        {centerSinglePanel && <div className="flex-1" />}
      </main>
    </>
  );
}
