import { detectFileType, isAnyTrashPath, pathMatches, remapPathPrefix } from './ideUtils.js';

export function isTabDirty(tab) {
  if (!tab) return false;
  return (tab.content || '') !== (tab.savedContent || '');
}

export function removeTabByPath(tabs, path) {
  return (tabs || []).filter((tab) => tab.path !== path);
}

export function nextActiveTabAfterClose(tabs, closedPath) {
  const nextTabs = removeTabByPath(tabs, closedPath);
  return nextTabs[nextTabs.length - 1] || null;
}

export function reorderTabsByPath(tabs, sourcePath, targetPath, position = 'before') {
  if (!sourcePath || !targetPath || sourcePath === targetPath) return tabs || [];
  const current = tabs || [];
  const sourceIndex = current.findIndex((tab) => tab.path === sourcePath);
  const targetIndex = current.findIndex((tab) => tab.path === targetPath);
  if (sourceIndex === -1 || targetIndex === -1) return current;

  const next = current.filter((tab) => tab.path !== sourcePath);
  const targetIndexAfterRemove = next.findIndex((tab) => tab.path === targetPath);
  if (targetIndexAfterRemove === -1) return current;
  const insertIndex = position === 'after' ? targetIndexAfterRemove + 1 : targetIndexAfterRemove;
  next.splice(insertIndex, 0, current[sourceIndex]);
  return next;
}

export function remapEditorTabs(tabs, oldPath, newPath) {
  return (tabs || []).map((tab) => {
    if (!pathMatches(tab.path, oldPath)) return tab;
    const nextPath = remapPathPrefix(tab.path, oldPath, newPath);
    return {
      ...tab,
      path: nextPath,
      fileType: detectFileType(nextPath),
    };
  });
}

export function remapEditorPath(path, oldPath, newPath) {
  if (!pathMatches(path, oldPath)) return path;
  return remapPathPrefix(path, oldPath, newPath);
}

export function filterTabsOutsidePath(tabs, removedPath) {
  return (tabs || []).filter((tab) => !pathMatches(tab.path, removedPath));
}

export function isSuccessfulEditorToolEvent(item) {
  return (
    item?.status === 'success' &&
    [
      'edit_workspace_text',
      'replace_workspace_lines',
      'save_workspace_file',
      'write_workspace_chunk',
      'finalize_workspace_write',
      'manage_workspace_path',
      'organize_workspace_matches',
      'run_workspace_command',
    ].includes(item?.tool_name)
  );
}

function isWorkspaceWriteTool(toolName) {
  return [
    'edit_workspace_text',
    'replace_workspace_lines',
    'save_workspace_file',
    'write_workspace_chunk',
    'finalize_workspace_write',
  ].includes(toolName);
}

export function pathMovesFromToolEvent(event) {
  if (!event) return [];
  if (event.old_path && event.new_path && !isAnyTrashPath(event.new_path)) {
    return [{ oldPath: event.old_path, newPath: event.new_path }];
  }
  if (!Array.isArray(event.changed_paths)) return [];

  const moves = [];
  for (let index = 0; index < event.changed_paths.length - 1; index += 2) {
    const oldPath = event.changed_paths[index];
    const newPath = event.changed_paths[index + 1];
    if (oldPath && newPath && !isAnyTrashPath(newPath)) {
      moves.push({ oldPath, newPath });
    }
  }
  return moves;
}

export function buildActiveFileToolPlan(events, { activePath = '', dirty = false } = {}) {
  let nextActivePath = activePath;
  let reloadPlan = null;
  let conflictPlan = null;
  let closeNotice = '';

  for (const event of events || []) {
    if (!nextActivePath) continue;

    if (event.tool_name === 'run_workspace_command') {
      if (dirty) {
        conflictPlan = {
          reloadPath: nextActivePath,
          hash: '',
          versionId: '',
          message: '命令可能修改了当前文件，但你有未保存内容，请选择保留本地修改或重新加载文件',
        };
      } else {
        reloadPlan = {
          path: nextActivePath,
          hash: '',
          versionId: '',
          statusMessage: '',
        };
      }
      continue;
    }

    if (isWorkspaceWriteTool(event.tool_name)) {
      if (event.path === nextActivePath || (event.changed_paths || []).includes(nextActivePath)) {
        if (dirty) {
          conflictPlan = {
            reloadPath: nextActivePath,
            hash: event.hash || '',
            versionId: event.version_id || '',
            message: '该文件已被 AI 修改，但你有未保存内容，请选择保留本地修改或重新加载文件',
          };
        } else {
          reloadPlan = {
            path: nextActivePath,
            hash: event.hash || '',
            versionId: event.version_id || '',
            statusMessage: '',
          };
        }
      }
      continue;
    }

    const moves = pathMovesFromToolEvent(event);
    if (moves.length > 0) {
      const matchedMove = moves.find((move) => pathMatches(nextActivePath, move.oldPath));
      if (matchedMove) {
        nextActivePath = remapEditorPath(nextActivePath, matchedMove.oldPath, matchedMove.newPath);
        const nextHash = event.hash || reloadPlan?.hash || conflictPlan?.hash || '';
        const nextVersionId = event.version_id || reloadPlan?.versionId || conflictPlan?.versionId || '';

        if (dirty) {
          conflictPlan = {
            reloadPath: nextActivePath,
            hash: nextHash,
            versionId: nextVersionId,
            message: '该文件已被 AI 修改，但你有未保存内容，请选择保留本地修改或重新加载文件',
          };
        } else {
          reloadPlan = {
            path: nextActivePath,
            hash: nextHash,
            versionId: nextVersionId,
            statusMessage: '',
          };
        }
      }
      continue;
    }

    if (event.tool_name === 'manage_workspace_path' && event.old_path && event.new_path && isAnyTrashPath(event.new_path)) {
      const deletedPath = event.old_path || event.path;
      if (pathMatches(nextActivePath, deletedPath)) {
        nextActivePath = '';
        reloadPlan = null;
        conflictPlan = null;
        closeNotice = '当前打开文件已被 AI 移入回收站';
      }
    }
  }

  return { activePath: nextActivePath, reloadPlan, conflictPlan, closeNotice };
}

export function buildInactiveTabWriteConflict(event) {
  return {
    reloadPath: event.path,
    hash: event.hash || '',
    versionId: event.version_id || '',
    message: '该文件已被 AI 修改，但这个标签页有未保存内容，请切换过去后选择保留本地修改或重新加载文件',
  };
}
