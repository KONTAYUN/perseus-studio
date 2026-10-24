import { apiGet } from '../api/client.js';
import { hasPendingDocumentSessions } from '../components/editor/document/registry.js';
import {
  buildActiveFileToolPlan,
  buildInactiveTabWriteConflict,
  deletedPathsFromToolEvent,
  fileMetadataFromToolEvent,
  filterTabsOutsidePath,
  isSuccessfulEditorToolEvent,
  isTabDirty,
  pathMovesFromToolEvent,
  remapEditorPath,
  remapEditorTabs,
} from './editorTabUtils.js';
import { detectFileType, isTextPath, isWorkspaceFileChangeEvent, pathMatches } from './ideUtils.js';

export function createEditorToolRefreshActions({
  closeCurrentFile,
  currentFilePathRef,
  isDirtyRef,
  loadFileIntoEditor,
  openTabsRef,
  openTabSnapshot,
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
      currentFilePathRef.current = nextFilePath;
      setCurrentFilePath(nextFilePath);
      setCurrentFileType(detectFileType(nextFilePath));
    }
    const nextTabs = remapEditorTabs(openTabsRef.current, oldPath, newPath);
    openTabsRef.current = nextTabs;
    setOpenTabs(() => nextTabs);
    setActiveTabPath((current) => remapEditorPath(current, oldPath, newPath));
  }

  function clearPathState(path) {
    if (!path) return;
    const previous = openTabsRef.current;
    const next = filterTabsOutsidePath(previous, path);
    const activeIndex = previous.findIndex(tab => tab.path === currentFilePathRef.current);
    openTabsRef.current = next;
    setOpenTabs(() => next);
    if (pathMatches(currentFilePathRef.current, path)) {
      const replacement = next[Math.min(Math.max(0, activeIndex), next.length - 1)];
      if (replacement) openTabSnapshot(replacement);
      else {
        closeCurrentFile();
        setEditorStatusMessage('当前文件已移入回收站');
      }
    }
  }

  async function refreshWorkspaceAfterAssistantTools(toolEvents) {
    const successfulEvents = (toolEvents || []).filter(isSuccessfulEditorToolEvent);
    if (successfulEvents.length === 0) return;

    const originalActivePath = currentFilePathRef.current;

    for (const event of successfulEvents) {
      const moves = pathMovesFromToolEvent(event);
      if (moves.length > 0) {
        for (const move of moves) {
          applyPathUpdate(move.oldPath, move.newPath);
        }
      }
      for (const path of deletedPathsFromToolEvent(event)) clearPathState(path);
    }
    const activeWasDeleted = successfulEvents.some(event => deletedPathsFromToolEvent(event).some(path => pathMatches(originalActivePath, path)));
    const activePath = activeWasDeleted ? currentFilePathRef.current : originalActivePath;
    const expectedPath = currentFilePathRef.current;
    for (const event of successfulEvents) await refreshMatchingOpenTabs(event);
    // A file read may finish after the user has selected another tab.
    if (!activePath || currentFilePathRef.current !== expectedPath) return;

    const { activePath: plannedActivePath, reloadPlan, conflictPlan, closeNotice } = buildActiveFileToolPlan(successfulEvents, {
      activePath,
      dirty: isDirtyRef.current || hasPendingDocumentSessions(originalActivePath),
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
        preserveLocalEdits: true,
        hash: reloadPlan.hash || '',
        versionId: reloadPlan.versionId || '',
        statusMessage: reloadPlan.statusMessage,
      });
    }
  }

  async function refreshMatchingOpenTabs(event) {
    if (!event) return;
    let tabs = openTabsRef.current;
    if (tabs.length === 0) return;
    for (const move of pathMovesFromToolEvent(event)) {
      tabs = remapEditorTabs(tabs, move.oldPath, move.newPath);
    }
    const deleted = deletedPathsFromToolEvent(event);
    tabs = tabs.filter((tab) => !deleted.some((path) => pathMatches(tab.path, path)));
    if (isWorkspaceFileChangedEvent(event) && (event.path || event.changed_paths?.length)) {
      const changedPaths = event.changed_paths?.length ? event.changed_paths : [event.path];
      const matching = tabs.filter((item) => changedPaths.includes(item.path) && item.path !== currentFilePathRef.current);
      await Promise.all(matching.map(async (tab) => {
        const metadata = fileMetadataFromToolEvent(event, tab.path);
        const conflict = buildInactiveTabWriteConflict({ ...event, path: tab.path, hash: metadata.hash, version_id: metadata.versionId });
        if (isTabDirty(tab) || hasPendingDocumentSessions(tab.path)) {
          setOpenTabs((current) => current.map((item) => item.path === tab.path ? { ...item, conflict } : item));
          return;
        }
        if (!isTextPath(tab.path)) return;
        try {
          const file = await apiGet(`/api/files/read?path=${encodeURIComponent(tab.path)}`);
          if (currentFilePathRef.current === tab.path) {
            setExternalFileConflict(conflict);
            setOpenTabs((current) => current.map((item) => item.path === tab.path ? { ...item, conflict } : item));
            return;
          }
          setOpenTabs((current) => current.map((item) => {
            if (item.path !== tab.path) return item;
            if (isTabDirty(item) || item.content !== tab.content || item.savedContent !== tab.savedContent) {
              return { ...item, conflict };
            }
            return {
              ...item, content: file.content, savedContent: file.content,
              hash: file.hash || '', versionId: metadata.versionId,
              statusMessage: '已自动刷新 AI 修改后的文件内容。',
            };
          }));
        } catch (error) {
          console.error('refreshMatchingOpenTabs failed', error);
        }
      }));
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
