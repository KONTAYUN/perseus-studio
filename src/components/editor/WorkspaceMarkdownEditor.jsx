import React from 'react';
import {
  MDXEditor,
  codeBlockPlugin,
  codeMirrorPlugin,
  headingsPlugin,
  imagePlugin,
  linkDialogPlugin,
  linkPlugin,
  listsPlugin,
  markdownShortcutPlugin,
  quotePlugin,
  tablePlugin,
  thematicBreakPlugin,
} from '@mdxeditor/editor';
import '@mdxeditor/editor/style.css';
import '../../styles/markdown-editor.css';
import { resolveWorkspaceMarkdownImage, resolveWorkspaceMarkdownLink } from './workspaceMarkdownUtils.js';
import { markdownContextualToolsPlugin } from './markdown/MarkdownContextualTools.jsx';
import { MarkdownLinkDialog } from './markdown/MarkdownLinkDialog.jsx';
import { CODE_BLOCK_LANGUAGES } from './markdown/markdownLanguages.js';

function isSameMarkdown(left, right) {
  return String(left ?? '') === String(right ?? '');
}

const MARKDOWN_EDITOR_TRANSLATIONS = {
  'contentArea.editableMarkdown': '可编辑 Markdown',
  'toolbar.bold': '加粗',
  'toolbar.removeBold': '取消加粗',
  'toolbar.italic': '斜体',
  'toolbar.removeItalic': '取消斜体',
  'toolbar.underline': '下划线',
  'toolbar.removeUnderline': '取消下划线',
  'toolbar.strikethrough': '删除线',
  'toolbar.removeStrikethrough': '取消删除线',
  'toolbar.link': '创建链接',
  'toolbar.image': '插入图片',
  'toolbar.table': '插入表格',
  'toolbar.codeBlock': '插入代码块',
  'toolbar.bulletedList': '无序列表',
  'toolbar.numberedList': '有序列表',
  'toolbar.checkList': '任务列表',
  'toolbar.toggleGroup': '列表类型',
  'toolbar.inlineCode': '行内代码',
  'toolbar.removeInlineCode': '取消行内代码',
  'toolbar.redo': '重做 {{shortcut}}',
  'toolbar.undo': '撤销 {{shortcut}}',
  'toolbar.blockTypes.paragraph': '段落',
  'toolbar.blockTypes.quote': '引用',
  'toolbar.blockTypes.heading': '标题 {{level}}',
  'toolbar.blockTypeSelect.selectBlockTypeTooltip': '选择块类型',
  'toolbar.blockTypeSelect.placeholder': '块类型',
  'toolbar.richText': '可视化',
  'toolbar.source': '源码',
  'toolbar.diffMode': '差异模式',
  'toolbar.removeHighlight': '取消高亮',
  'toolbar.subscript': '下标',
  'toolbar.removeSubscript': '取消下标',
  'toolbar.superscript': '上标',
  'toolbar.removeSuperscript': '取消上标',
  'toolbar.admonition': '提示块',
  'toolbar.thematicBreak': '分隔线',
  'toolbar.editFrontmatter': '编辑文档元数据',
  'codeBlock.language': '代码语言',
  'codeBlock.selectLanguage': '选择代码语言',
  'codeBlock.inlineLanguage': '语言',
  'codeblock.delete': '删除代码块',
  'createLink.url': '链接地址',
  'createLink.urlPlaceholder': '粘贴或输入链接地址',
  'createLink.text': '链接文本',
  'createLink.textTooltip': '显示为链接的文本',
  'createLink.title': '链接标题',
  'createLink.titleTooltip': '鼠标悬停时显示的链接标题',
  'createLink.saveTooltip': '保存链接',
  'createLink.cancelTooltip': '取消编辑链接',
  'linkPreview.open': '在新窗口打开 {{url}}',
  'linkPreview.edit': '编辑链接地址',
  'linkPreview.copyToClipboard': '复制链接地址',
  'linkPreview.copied': '已复制',
  'linkPreview.remove': '移除链接',
  'uploadImage.dialogTitle': '插入图片',
  'uploadImage.uploadInstructions': '从设备上传图片',
  'uploadImage.addViaUrlInstructions': '或通过图片地址添加',
  'uploadImage.addViaUrlInstructionsNoUpload': '通过图片地址添加',
  'uploadImage.autoCompletePlaceholder': '粘贴图片地址或选择图片',
  'uploadImage.alt': '替代文本',
  'uploadImage.title': '图片标题',
  'uploadImage.width': '宽度',
  'uploadImage.height': '高度',
  'imageEditor.editImage': '编辑图片',
  'imageEditor.deleteImage': '删除图片',
  'dialogControls.save': '保存',
  'dialogControls.cancel': '取消',
  'dialog.close': '关闭对话框',
  'table.deleteTable': '删除表格',
  'table.columnMenu': '列操作',
  'table.rowMenu': '行操作',
  'table.textAlignment': '文字对齐',
  'table.alignLeft': '左对齐',
  'table.alignCenter': '居中对齐',
  'table.alignRight': '右对齐',
  'table.insertColumnLeft': '在左侧插入列',
  'table.insertColumnRight': '在右侧插入列',
  'table.deleteColumn': '删除此列',
  'table.insertRowAbove': '在上方插入行',
  'table.insertRowBelow': '在下方插入行',
  'table.deleteRow': '删除此行',
};

function translateMarkdownEditor(key, defaultValue, interpolations = {}) {
  let value = MARKDOWN_EDITOR_TRANSLATIONS[key] ?? defaultValue;
  for (const [name, replacement] of Object.entries(interpolations)) {
    value = value.replaceAll(`{{${name}}}`, String(replacement));
  }
  return value;
}

// MDXEditor treats `markdown` as its initial value and exposes setMarkdown for
// later synchronization. Ignoring that prop in the child comparator prevents
// every user keystroke from re-running RealmWithPlugins' update lifecycle;
// WorkspaceMarkdownEditor still applies genuine external changes through the
// imperative sync effect above.
const StableMarkdownEditor = React.memo(
  MDXEditor,
  (previous, next) => previous.readOnly === next.readOnly
    && previous.plugins === next.plugins
    && previous.onChange === next.onChange
    && previous.onError === next.onError
    && previous.translation === next.translation
    && previous.className === next.className
    && previous.contentEditableClassName === next.contentEditableClassName
    && previous.overlayContainer === next.overlayContainer
    && previous.trim === next.trim
    && previous.suppressHtmlProcessing === next.suppressHtmlProcessing
);

function WorkspaceMarkdownEditor({
  path,
  content = '',
  readOnly = false,
  scrollTop = 0,
  onScrollTopChange,
  onContentChange,
  onStatusMessage,
  onImageUpload,
  onOpenFile,
}) {
  const editorRef = React.useRef(null);
  const scrollRef = React.useRef(null);
  const [overlayContainer, setOverlayContainer] = React.useState(null);
  const scrollTopRef = React.useRef(Number(scrollTop || 0));
  const scrollCommitTimerRef = React.useRef(null);
  const lastEditorMarkdownRef = React.useRef(String(content ?? ''));
  const lastExternalMarkdownRef = React.useRef(String(content ?? ''));
  const syncingExternalMarkdownRef = React.useRef(false);
  const lastPathRef = React.useRef(path);
  const pathRef = React.useRef(path);
  const readOnlyRef = React.useRef(readOnly);
  const callbacksRef = React.useRef({
    onContentChange,
    onImageUpload,
    onOpenFile,
    onScrollTopChange,
    onStatusMessage,
  });
  const [parseError, setParseError] = React.useState('');

  const handleOverlayContainerRef = React.useCallback((element) => {
    setOverlayContainer((current) => (current === element ? current : element));
  }, []);

  pathRef.current = path;
  readOnlyRef.current = readOnly;
  callbacksRef.current = {
    onContentChange,
    onImageUpload,
    onOpenFile,
    onScrollTopChange,
    onStatusMessage,
  };

  const flushScrollPosition = React.useCallback(() => {
    if (scrollCommitTimerRef.current) {
      window.clearTimeout(scrollCommitTimerRef.current);
      scrollCommitTimerRef.current = null;
    }
    callbacksRef.current.onScrollTopChange?.(scrollTopRef.current);
  }, []);

  React.useEffect(() => () => flushScrollPosition(), [flushScrollPosition]);

  React.useLayoutEffect(() => {
    if (!scrollRef.current) return;
    scrollTopRef.current = Number(scrollTop || 0);
    scrollRef.current.scrollTop = scrollTopRef.current;
  }, [path]);

  React.useEffect(() => {
    const nextMarkdown = String(content ?? '');
    const pathChanged = lastPathRef.current !== path;
    lastPathRef.current = path;
    if (
      !editorRef.current ||
      (!pathChanged &&
        (isSameMarkdown(nextMarkdown, lastEditorMarkdownRef.current) ||
          isSameMarkdown(nextMarkdown, lastExternalMarkdownRef.current)))
    ) {
      return;
    }
    syncingExternalMarkdownRef.current = true;
    lastExternalMarkdownRef.current = nextMarkdown;
    lastEditorMarkdownRef.current = nextMarkdown;
    editorRef.current.setMarkdown(nextMarkdown);
    queueMicrotask(() => {
      syncingExternalMarkdownRef.current = false;
    });
  }, [content, path]);

  const handleChange = React.useCallback((nextMarkdown, initialMarkdownNormalize) => {
    lastEditorMarkdownRef.current = nextMarkdown;
    if (syncingExternalMarkdownRef.current || initialMarkdownNormalize || readOnlyRef.current) return;
    callbacksRef.current.onContentChange?.(nextMarkdown);
  }, []);

  const handleImagePreview = React.useCallback(
    async (source) => resolveWorkspaceMarkdownImage(pathRef.current, source),
    []
  );

  const handleImageUpload = React.useCallback((file) => {
    const upload = callbacksRef.current.onImageUpload;
    if (!upload) return Promise.reject(new Error('Markdown 图片上传不可用。'));
    return upload(file);
  }, []);

  const plugins = React.useMemo(
    () => [
      headingsPlugin(),
      listsPlugin(),
      quotePlugin(),
      thematicBreakPlugin(),
      linkPlugin(),
      tablePlugin(),
      imagePlugin({
        imageUploadHandler: handleImageUpload,
        imagePreviewHandler: handleImagePreview,
      }),
      linkDialogPlugin({ LinkDialog: MarkdownLinkDialog }),
      codeBlockPlugin({ defaultCodeBlockLanguage: 'text' }),
      codeMirrorPlugin({
        codeBlockLanguages: CODE_BLOCK_LANGUAGES,
        autoLoadLanguageSupport: true,
      }),
      markdownShortcutPlugin(),
      markdownContextualToolsPlugin(),
    ],
    [handleImagePreview, handleImageUpload]
  );

  const handleClick = React.useCallback((event) => {
    const anchor = event.target.closest?.('a');
    if (!anchor) return;
    const target = resolveWorkspaceMarkdownLink(pathRef.current, anchor.getAttribute('href') || '');
    if (target?.external) {
      anchor.setAttribute('target', '_blank');
      anchor.setAttribute('rel', 'noopener noreferrer');
      return;
    }
    if (!target?.workspacePath) return;
    event.preventDefault();
    callbacksRef.current.onOpenFile?.(target.workspacePath);
  }, []);

  const handleParseError = React.useCallback(({ error }) => {
    const message = String(error || 'Markdown 无法渲染。');
    setParseError((current) => (current === message ? current : message));
    callbacksRef.current.onStatusMessage?.(message, { tone: 'warning' });
  }, []);

  const handleScroll = React.useCallback((event) => {
    scrollTopRef.current = event.currentTarget.scrollTop;
    if (scrollCommitTimerRef.current) window.clearTimeout(scrollCommitTimerRef.current);
    scrollCommitTimerRef.current = window.setTimeout(() => {
      scrollCommitTimerRef.current = null;
      callbacksRef.current.onScrollTopChange?.(scrollTopRef.current);
    }, 120);
  }, []);

  return (
    <div
      ref={handleOverlayContainerRef}
      className="workspace-markdown-editor-overlay-root h-full min-h-0"
    >
      <div
        ref={scrollRef}
        data-testid="markdown-editor"
        className="workspace-markdown-editor-scroll workspace-markdown-editor-host custom-scrollbar h-full min-h-0 overflow-y-auto"
        onScroll={handleScroll}
        onClick={handleClick}
      >
        {parseError && (
          <div className="workspace-markdown-editor-warning" role="status">
            当前文档的 Markdown 预览能力有限，请切换到源码模式编辑不支持的语法。
          </div>
        )}
        <StableMarkdownEditor
          ref={editorRef}
          markdown={String(content ?? '')}
          onChange={handleChange}
          onError={handleParseError}
          plugins={plugins}
          readOnly={readOnly}
          trim={false}
          suppressHtmlProcessing
          translation={translateMarkdownEditor}
          className="workspace-markdown-editor mdxeditor mdxeditor-full-height"
          contentEditableClassName="workspace-markdown-editor-content"
          overlayContainer={overlayContainer}
        />
      </div>
    </div>
  );
}

export default React.memo(WorkspaceMarkdownEditor);
