import React, { useEffect, useRef, useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { Button } from '../ui';

export function CodeCopyButton({ text }) {
  const [status, setStatus] = useState('');
  const timer = useRef(null);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  async function copy(event) {
    event.stopPropagation();
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(text);
      setStatus('已复制');
    } catch {
      setStatus('复制失败，请选中复制');
    }
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setStatus(''), 2000);
  }
  return (
    <Button type="button" variant="ghost" size="xs" className="code-copy-button" onClick={copy} aria-label="复制代码">
      {status === '已复制' ? <Check size={13} /> : <Copy size={13} />}
      <span aria-live="polite">{status || '复制'}</span>
    </Button>
  );
}

export default function CodeBlock({ text, language = '' }) {
  return (
    <div className="code-fence" data-testid="markdown-code-block">
      <div className="code-fence-header">
        <span>{language || '纯文本'}</span>
        <CodeCopyButton text={text} />
      </div>
      <pre className="custom-scrollbar"><code className={language ? `language-${language}` : undefined}>{text}</code></pre>
    </div>
  );
}
