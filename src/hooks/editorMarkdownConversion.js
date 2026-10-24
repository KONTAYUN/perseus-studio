// Conversion preserves the source; Markdown rendering never rewrites the document here.
export function createMarkdownConversionAction({
  conversionRequestRef, autoSaveTimerRef, currentFilePathRef, currentFileContentRef,
  savedFileContentRef, currentFileHashRef, externalFileConflictRef, isTextFileRef,
  saveFileContent, renamePath, setIsConvertingFile, setErrorMessage, setDocumentModes,
}) {
  return function convertToMarkdown() {
    if (conversionRequestRef.current) return conversionRequestRef.current;
    const path = currentFilePathRef.current;
    const content = currentFileContentRef.current;
    const matches = () => currentFilePathRef.current === path && currentFileContentRef.current === content
      && isTextFileRef.current && !externalFileConflictRef.current;
    if (!/\.txt$/i.test(path) || !matches()) return Promise.resolve(false);
    const target = path.replace(/\.txt$/i, '.md');
    clearTimeout(autoSaveTimerRef.current);
    autoSaveTimerRef.current = null;
    setIsConvertingFile(true);
    setErrorMessage?.('');
    const request = Promise.resolve().then(async () => {
      await saveFileContent({ auto: true, conversion: true });
      if (!matches() || savedFileContentRef.current !== content || !currentFileHashRef.current) {
        throw new Error('当前文件已变化或尚未保存，已停止转换，请确认后重试。');
      }
      await renamePath(path, target, { expectedHash: currentFileHashRef.current });
      setDocumentModes?.(modes => ({ ...modes, [target]: 'rich' }));
      return true;
    }).catch(error => {
      setErrorMessage?.(`转换为 Markdown 失败：${error.message}`);
      return false;
    }).finally(() => {
      conversionRequestRef.current = null;
      setIsConvertingFile(false);
    });
    conversionRequestRef.current = request;
    return request;
  };
}
