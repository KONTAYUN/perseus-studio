import { apiGet } from '../api/client';
import {
  buildActiveFileToolPlan,
  buildInactiveTabWriteConflict,
  filterTabsOutsidePath,
  isSuccessfulEditorToolEvent,
  isTabDirty,
  pathMovesFromToolEvent,
  remapEditorPath,
  remapEditorTabs,
} from './editorTabUtils';
import { detectFileType, isAnyTrashPath, isTextPath, pathMatches } from './ideUtils';

export function createEditorToolRefreshActions({
  closeCurrentFile,
  currentFilePathRef,
  isDirtyRef,
  loadFileIntoEditor,
  openTabsRef,
  setActiveTabPath,
  setCurrentFilePath,
  setCurrentFileType,
  setEditorStatusMessage,
  setExternalFileConflict,
  setOpenTabs,
}) {
  function applyPathUpdate(oldPath, newPath) {
    if (!oldPath || !newPath) return;

    if (pathMatches(currentFilePathRef.current, oldPath)) {
      const nextFilePath = remapEditorPath(currentFilePathRef.current, oldPath, newPath);
      setCurrentFilePath(nextFilePath);
      setCurrentFileType(detectFileType(nextFilePath));
    }
    setOpenTabs((current) => remapEditorTabs(current, oldPath, newPath));
    setActiveTabPath((current) => remapEditorPath(current, oldPath, newPath));
  }

  function clearPathState(path) {
    if (!path) return;
    setOpenTabs((current) => filterTabsOutsidePath(current, path));
    if (pathMatches(currentFilePathRef.current, path)) {
      closeCurrentFile();
      setEditorStatusMessage('当前文件已移入回收站');
    }
  }

  async function refreshWorkspaceAfterAssistantTools(toolEvents) {
    const successfulEvents = (toolEvents || []).filter(isSuccessfulEditorToolEvent);
    if (successfulEvents.length === 0) return;

    const activePath = currentFilePathRef.current;
    const dirty = isDirtyRef.current;

    for (const event of successfulEvents) {
      const moves = pathMovesFromToolEvent(event);
      if (moves.length > 0) {
        for (const move of moves) {
          applyPathUpdate(move.oldPath, move.newPath);
        }
      } else if (event.tool_name === 'manage_workspace_path' && isAnyTrashPath(event.new_path)) {
        clearPathState(event.old_path || event.path);
      }
      await refreshMatchingOpenTabs(event);
    }

    const { activePath: plannedActivePath, reloadPlan, conflictPlan, closeNotice } = buildActiveFileToolPlan(successfulEvents, {
      activePath,
      dirty,
    });
    if (plannedActivePath && plannedActivePath !== activePath) {
      setCurrentFilePath(plannedActivePath);
      setCurrentFileType(detectFileType(plannedActivePath));
    }

    if (closeNotice) {
      closeCurrentFile();
      setEditorStatusMessage(closeNotice);
      return;
    }

    if (conflictPlan) {
      setExternalFileConflict(conflictPlan);
      setEditorStatusMessage('');
      return;
    }

    if (reloadPlan?.path) {
      await loadFileIntoEditor(reloadPlan.path, {
        preserveSelection: true,
        hash: reloadPlan.hash || '',
        versionId: reloadPlan.versionId || '',
        statusMessage: reloadPlan.statusMessage,
      });
    }
  }

  async function refreshMatchingOpenTabs(event) {
    if (!event?.tool_name) return;
    const tabs = openTabsRef.current;
    if (tabs.length === 0) return;
    if (event.tool_name === 'manage_workspace_path' || event.tool_name === 'organize_workspace_matches') {
      const moves = pathMovesFromToolEvent(event);
      if (moves.length > 0) {
        setOpenTabs((current) =>
          current.map((item) => {
            const matchedMove = moves.find((move) => pathMatches(item.path, move.oldPath));
            if (!matchedMove) return item;
            return {
              ...item,
              path: remapEditorPath(item.path, matchedMove.oldPath, matchedMove.newPath),
              fileType: detectFileType(remapEditorPath(item.path, matchedMove.oldPath, matchedMove.newPath)),
            };
          })
        );
      }
    }
    if ((event.tool_name === 'edit_workspace_text' || event.tool_name === 'save_workspace_file') && event.path) {
      const tab = tabs.find((item) => item.path === event.path && item.path !== currentFilePathRef.current);
      if (!tab) return;
      const dirty = isTabDirty(tab);
      if (dirty) {
        setOpenTabs((current) =>
          current.map((item) =>
            item.path === event.path
              ? {
                  ...item,
                  conflict: buildInactiveTabWriteConflict(event),
                }
              : item
          )
        );
        return;
      }
      if (!isTextPath(event.path)) return;
      try {
        const file = await apiGet(`/api/files/read?path=${encodeURIComponent(event.path)}`);
        setOpenTabs((current) =>
          current.map((item) =>
            item.path === event.path
              ? {
                  ...item,
                  content: file.content,
                  savedContent: file.content,
                  hash: event.hash || file.hash || '',
                  versionId: event.version_id || item.versionId || '',
                  statusMessage: '已自动刷新 AI 修改后的文件内容。',
                }
              : item
          )
        );
      } catch (error) {
        console.error('refreshMatchingOpenTabs failed', error);
      }
    }
  }

  return {
    applyPathUpdate,
    clearPathState,
    refreshWorkspaceAfterAssistantTools,
  };
}
