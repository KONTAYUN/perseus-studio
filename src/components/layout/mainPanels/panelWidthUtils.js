import { minWidthForPanelType } from '../../../hooks/useLayoutState';

export const PANEL_GAP = 12;
export const MAIN_HORIZONTAL_PADDING = 24;

export function isCenteredSingleChatPanel(panels = []) {
  return panels.length === 1 && panels[0]?.type === 'chat';
}

export function defaultWidthForPanel(panel, { centerSinglePanel = false, contentWidth = 0 } = {}) {
  if (panel.type === 'fileTree') return 300;
  if (panel.type === 'chat') {
    return centerSinglePanel ? Math.min(1120, Math.max(760, contentWidth)) : 500;
  }
  return 520;
}

export function calculateDisplayWidths({
  mainWidth = 0,
  panels = [],
  widths = {},
  panelGap = PANEL_GAP,
} = {}) {
  const centerSinglePanel = isCenteredSingleChatPanel(panels);
  const contentWidth = Math.max(0, mainWidth - MAIN_HORIZONTAL_PADDING);
  if (mainWidth <= 0 || panels.length === 0) return {};

  const gapTotal = Math.max(0, panels.length - 1) * panelGap;
  const minimumTotal = panels.reduce((sum, panel) => sum + minWidthForPanelType(panel.type), 0);
  const available = Math.max(minimumTotal, contentWidth - gapTotal);
  const entries = panels.map((panel) => ({
    id: panel.id,
    type: panel.type,
    width:
      centerSinglePanel && panel.type === 'chat'
        ? defaultWidthForPanel(panel, { centerSinglePanel, contentWidth })
        : widths[panel.id] || defaultWidthForPanel(panel, { centerSinglePanel, contentWidth }),
  }));
  let total = entries.reduce((sum, item) => sum + item.width, 0);

  if (total > available) {
    const overflow = total - available;
    const shrinkable = entries.map((item) => Math.max(0, item.width - minWidthForPanelType(item.type)));
    const shrinkableTotal = shrinkable.reduce((sum, value) => sum + value, 0);
    entries.forEach((item, index) => {
      const share = shrinkableTotal > 0 ? overflow * (shrinkable[index] / shrinkableTotal) : overflow / entries.length;
      item.width = Math.max(minWidthForPanelType(item.type), item.width - share);
    });
  } else if (!centerSinglePanel) {
    const growTargets = entries.filter((item) => item.type === 'editor');
    const targets = growTargets.length > 0 ? growTargets : entries;
    const extra = (available - total) / targets.length;
    targets.forEach((item) => {
      item.width += extra;
    });
  }

  return Object.fromEntries(entries.map((item) => [item.id, Math.round(item.width)]));
}

export function collectPanelSizes(panels = [], displayWidths = {}) {
  return Object.fromEntries(
    panels
      .map((panel) => {
        const element = document.getElementById(`panel-wrapper-${panel.id}`);
        return [panel.id, Math.round(element?.getBoundingClientRect().width || displayWidths[panel.id] || 0)];
      })
      .filter(([, width]) => width > 0)
  );
}
