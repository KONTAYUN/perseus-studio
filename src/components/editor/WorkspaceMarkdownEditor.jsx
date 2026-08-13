import React from 'react';
import {
  BlockTypeSelect,
  BoldItalicUnderlineToggles,
  ChangeCodeMirrorLanguage,
  ConditionalContents,
  CreateLink,
  InsertCodeBlock,
  InsertImage,
  InsertTable,
  MDXEditor,
  Separator,
  ListsToggle,
  UndoRedo,
  codeBlockPlugin,
  codeMirrorPlugin,
  headingsPlugin,
  imagePlugin,
  linkPlugin,
  listsPlugin,
  markdownShortcutPlugin,
  quotePlugin,
  tablePlugin,
  thematicBreakPlugin,
  toolbarPlugin,
} from '@mdxeditor/editor';
import '@mdxeditor/editor/style.css';
import { resolveWorkspaceMarkdownImage, resolveWorkspaceMarkdownLink } from './workspaceMarkdownUtils.js';

const CODE_BLOCK_LANGUAGES = {
  text: 'Text',
  bash: 'Bash',
  shell: 'Shell',
  json: 'JSON',
  yaml: 'YAML',
  javascript: 'JavaScript',
  typescript: 'TypeScript',
  python: 'Python',
  html: 'HTML',
  css: 'CSS',
  sql: 'SQL',
};

function isSameMarkdown(left, right) {
  return String(left ?? '') === String(right ?? '');
}

function ToolbarGroup({ children, className = '' }) {
  return <div className={`workspace-markdown-toolbar-group ${className}`.trim()}>{children}</div>;
}

export default function WorkspaceMarkdownEditor({
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
  const lastEditorMarkdownRef = React.useRef(String(content ?? ''));
  const lastExternalMarkdownRef = React.useRef(String(content ?? ''));
  const syncingExternalMarkdownRef = React.useRef(false);
  const lastPathRef = React.useRef(path);
  const [parseError, setParseError] = React.useState('');

  React.useLayoutEffect(() => {
    if (!scrollRef.current) return;
    scrollRef.current.scrollTop = Number(scrollTop || 0);
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
    if (syncingExternalMarkdownRef.current || initialMarkdownNormalize || readOnly) return;
    onContentChange?.(nextMarkdown);
  }, [onContentChange, readOnly]);

  const handleImagePreview = React.useCallback(
    async (source) => resolveWorkspaceMarkdownImage(path, source),
    [path]
  );

  const plugins = React.useMemo(() => [
    headingsPlugin(),
    listsPlugin(),
    quotePlugin(),
    thematicBreakPlugin(),
    linkPlugin(),
    tablePlugin(),
    imagePlugin({
      imageUploadHandler: onImageUpload || null,
      imagePreviewHandler: handleImagePreview,
    }),
    codeBlockPlugin({ defaultCodeBlockLanguage: 'text' }),
    codeMirrorPlugin({
      codeBlockLanguages: CODE_BLOCK_LANGUAGES,
      autoLoadLanguageSupport: true,
    }),
    markdownShortcutPlugin(),
    toolbarPlugin({
      toolbarContents: () => (
        <>
          <ToolbarGroup className="workspace-markdown-toolbar-history">
            <UndoRedo />
          </ToolbarGroup>
          <Separator />
          <ToolbarGroup className="workspace-markdown-toolbar-block-type">
            <BlockTypeSelect />
          </ToolbarGroup>
          <Separator />
          <ToolbarGroup>
            <BoldItalicUnderlineToggles />
          </ToolbarGroup>
          <ToolbarGroup>
            <ListsToggle />
          </ToolbarGroup>
          <Separator />
          <ToolbarGroup>
            <CreateLink />
            <InsertImage />
            <InsertTable />
            <InsertCodeBlock />
          </ToolbarGroup>
          <ConditionalContents
            options={[
              {
                when: (editorInFocus) => editorInFocus?.editorType === 'codeblock',
                contents: () => <ChangeCodeMirrorLanguage />,
              },
            ]}
          />
        </>
      ),
    }),
  ], [handleImagePreview, onImageUpload]);

  const handleClick = React.useCallback((event) => {
    const anchor = event.target.closest?.('a');
    if (!anchor) return;
    const target = resolveWorkspaceMarkdownLink(path, anchor.getAttribute('href') || '');
    if (target?.external) {
      anchor.setAttribute('target', '_blank');
      anchor.setAttribute('rel', 'noopener noreferrer');
      return;
    }
    if (!target?.workspacePath) return;
    event.preventDefault();
    onOpenFile?.(target.workspacePath);
  }, [onOpenFile, path]);

  const handleParseError = React.useCallback(({ error }) => {
    const message = String(error || 'Markdown could not be rendered.');
    setParseError(message);
    onStatusMessage?.(message, { tone: 'warning' });
  }, [onStatusMessage]);

  return (
    <div
      ref={scrollRef}
      data-testid="markdown-editor"
      className="workspace-markdown-editor-scroll custom-scrollbar h-full min-h-0 overflow-y-auto"
      onScroll={(event) => onScrollTopChange?.(event.currentTarget.scrollTop)}
      onClick={handleClick}
    >
      {parseError && (
        <div className="workspace-markdown-editor-warning" role="status">
          Markdown preview is limited for this document. Use Source mode to edit unsupported syntax.
        </div>
      )}
      <MDXEditor
        ref={editorRef}
        markdown={String(content ?? '')}
        onChange={handleChange}
        onError={handleParseError}
        plugins={plugins}
        readOnly={readOnly}
        trim={false}
        suppressHtmlProcessing
        className="workspace-markdown-editor mdxeditor-full-height"
        contentEditableClassName="workspace-markdown-editor-content"
      />
    </div>
  );
}
