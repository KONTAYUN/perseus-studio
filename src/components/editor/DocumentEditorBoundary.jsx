import React from 'react';
import { Button } from '../ui';

// Component failures are not document damage. Never send an ordinary user to
// raw Markdown, or overwrite the authoritative file with a broken rendering.
export default class DocumentEditorBoundary extends React.Component {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() {
    if (!this.state.failed) return this.props.children;
    return <div className="rich-document-editor">
      <div className="rich-section-toolbar" role="alert">编辑器暂时无法打开，原文件不会被此故障覆盖。请重新加载；若有未完成的编辑，重新打开后可检查浏览器草稿。</div>
      <Button variant="secondary" onClick={() => window.location.reload()}>重新加载编辑器</Button>
    </div>;
  }
}
