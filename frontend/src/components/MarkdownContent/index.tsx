import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import hljs from 'highlight.js';
import { Button, Tooltip } from 'antd';
import { CopyOutlined, CheckOutlined } from '@ant-design/icons';
import 'highlight.js/styles/github-dark.css';

interface MarkdownContentProps {
  content: string;
}

function CodeBlock({ language, code }: { language: string; code: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard unavailable */
    }
  };

  let highlighted = '';
  if (language && hljs.getLanguage(language)) {
    try {
      highlighted = hljs.highlight(code, { language }).value;
    } catch {
      highlighted = hljs.highlightAuto(code).value;
    }
  } else {
    highlighted = hljs.highlightAuto(code).value;
  }

  return (
    <div className="markdown-code-block">
      <div className="markdown-code-header">
        <span className="markdown-code-lang">{language || 'code'}</span>
        <Tooltip title={copied ? '已复制' : '复制代码'}>
          <Button
            size="small"
            type="text"
            icon={copied ? <CheckOutlined style={{ color: '#10B981' }} /> : <CopyOutlined />}
            onClick={handleCopy}
            className="markdown-code-copy"
          />
        </Tooltip>
      </div>
      <pre>
        {/* eslint-disable-next-line react/no-danger */}
        <code dangerouslySetInnerHTML={{ __html: highlighted }} />
      </pre>
    </div>
  );
}

export default function MarkdownContent({ content }: MarkdownContentProps) {
  return (
    <div className="markdown-body">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          code({ className, children, ...props }) {
            const match = /language-(\w+)/.exec(className || '');
            const isInline = !match && !String(children).includes('\n');
            const code = String(children).replace(/\n$/, '');
            if (isInline) {
              return <code className="markdown-inline-code" {...props}>{children}</code>;
            }
            return <CodeBlock language={match?.[1] ?? ''} code={code} />;
          },
          table({ children }) {
            return (
              <div className="markdown-table-wrap">
                <table className="markdown-table">{children}</table>
              </div>
            );
          },
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}
