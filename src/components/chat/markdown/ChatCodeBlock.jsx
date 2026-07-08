import React from 'react';
import { Check, Copy } from 'lucide-react';
import { IconButton } from '../../ui';
import {
  displayLanguageName,
  languageFromClassName,
  normalizeCodeForCopy,
  reactNodeToPlainText,
} from './chatMarkdownUtils';

function ChatCodeBlock({ className = '', children }) {
  const [copied, setCopied] = React.useState(false);
  const timeoutRef = React.useRef(null);
  const language = languageFromClassName(className);
  const label = displayLanguageName(language);
  const code = normalizeCodeForCopy(reactNodeToPlainText(children));

  React.useEffect(() => () => window.clearTimeout(timeoutRef.current), []);

  async function handleCopy(event) {
    event.stopPropagation();
    try {
      await navigator.clipboard?.writeText(code);
      setCopied(true);
      window.clearTimeout(timeoutRef.current);
      timeoutRef.current = window.setTimeout(() => setCopied(false), 1400);
    } catch (error) {
      console.error(error);
    }
  }

  return (
    <div className="chat-code-block motion-list-enter">
      <div className="chat-code-block-header">
        <span className="chat-code-block-language">{label}</span>
        <IconButton
          type="button"
          size="compact"
          title={copied ? '已复制代码' : '复制代码'}
          label={copied ? '已复制代码' : '复制代码'}
          onClick={handleCopy}
          className={`chat-code-copy-button ${copied ? 'motion-copy-confirm' : ''}`}
        >
          {copied ? <Check size={13} /> : <Copy size={13} />}
        </IconButton>
      </div>
      <pre className="chat-code-block-body custom-scrollbar">
        <code className={className}>{children}</code>
      </pre>
    </div>
  );
}

export default React.memo(ChatCodeBlock);
