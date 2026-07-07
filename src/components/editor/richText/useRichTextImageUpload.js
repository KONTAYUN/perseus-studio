import React from 'react';
import { apiUploadFile, workspaceFileContentUrl } from '../../../api/client';
import {
  buildNoteAssetUploadPath,
  getNoteAssetReferencePath,
} from '../../../hooks/noteTreeUtils.js';

export default function useRichTextImageUpload({ editor, filePath, onStatusMessage }) {
  return React.useCallback(
    async (file) => {
      if (!file || !editor || !filePath) return false;
      const uploadPath = buildNoteAssetUploadPath(filePath, file.name);
      const referencePath = getNoteAssetReferencePath(filePath, uploadPath);
      try {
        onStatusMessage?.('正在上传图片...');
        await apiUploadFile(uploadPath, file, false);
        editor
          .chain()
          .focus()
          .setImage({
            alt: file.name,
            src: workspaceFileContentUrl(uploadPath),
            'data-note-src': referencePath,
            align: 'center',
            widthPercent: '100',
          })
          .run();
        onStatusMessage?.('图片已插入笔记');
        return true;
      } catch (error) {
        console.error('insert image failed', error);
        onStatusMessage?.(`图片上传失败：${error.message}`);
        return false;
      }
    },
    [editor, filePath, onStatusMessage]
  );
}
