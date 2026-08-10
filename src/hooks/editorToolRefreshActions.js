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
import { detectFileType, isAnyTrashPath, isTextPath, isWorkspaceFileChangeEvent, pathMatches } from './ideUtils';

export function createEditorToolRefreshActions({
  autoSaveTimerRef,
  closeCurrentFile,
  currentFilePathRef,
  isDirtyRef,
  applyAuthoritativeFileSnapshot,
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
      } else if ((event.operation === 'delete' || event.operation === 'trash') && isAnyTrashPath(event.new_path)) {
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
      const authoritativeEvent = successfulEvents.find((event) =>
        event.authoritative === true &&
        (event.path === reloadPlan.path || event.changed_paths?.includes(reloadPlan.path))
      );
      if (authoritativeEvent && autoSaveTimerRef.current) {
        window.clearTimeout(autoSaveTimerRef.current);
        autoSaveTimerRef.current = null;
      }
      if (authoritativeEvent && isTextPath(reloadPlan.path)) {
        const file = await apiGet(`/api/files/read?path=${encodeURIComponent(reloadPlan.path)}`);
        applyAuthoritativeFileSnapshot(reloadPlan.path, file, authoritativeEvent);
      } else {
        await loadFileIntoEditor(reloadPlan.path, {
          preserveSelection: true,
          hash: reloadPlan.hash || '',
          versionId: reloadPlan.versionId || '',
          statusMessage: reloadPlan.statusMessage,
        });
      }
    }
  }

  async function refreshMatchingOpenTabs(event) {
    if (!event) return;
    const tabs = openTabsRef.current;
    if (tabs.length === 0) return;
    if (event.operation === 'move' || event.operation === 'rename' || event.operation === 'organize') {
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
    if (isWorkspaceFileChangedEvent(event) && (event.path || event.changed_paths?.length)) {
      const changedPaths = event.changed_paths?.length ? event.changed_paths : [event.path];
      const tab = tabs.find((item) => changedPaths.includes(item.path) && item.path !== currentFilePathRef.current);
      if (!tab) return;
      const dirty = isTabDirty(tab);
      if (dirty) {
        setOpenTabs((current) =>
          current.map((item) =>
            item.path === tab.path
              ? {
                  ...item,
                  conflict: buildInactiveTabWriteConflict({ ...event, path: tab.path }),
                }
              : item
          )
        );
        return;
      }
      if (!isTextPath(tab.path)) return;
      try {
        const file = await apiGet(`/api/files/read?path=${encodeURIComponent(tab.path)}`);
        setOpenTabs((current) =>
          current.map((item) =>
            item.path === tab.path
              ? {
                  ...item,
                  content: file.content,
                  savedContent: file.content,
                  hash: event.hash || file.hash || '',
                  versionId: event.version_id || item.versionId || '',
                  documentVersion: event.document_version || file.document_version || file.hash || '',
                  userRevision: item.userRevision || 0,
                  savedUserRevision: item.userRevision || 0,
                  aiPreview: null,
                  previewOnly: false,
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

function isWorkspaceFileChangedEvent(event) {
  return isWorkspaceFileChangeEvent(event);
}
