import ReactMarkdown from 'react-markdown'
import rehypeHighlight from 'rehype-highlight'
import remarkGfm from 'remark-gfm'

export default function MarkdownText({ text }: { text: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown
        components={{
          a: ({ href, children, ...props }) => (
            <a href={href} target="_blank" rel="noreferrer" {...props}>
              {children}
            </a>
          ),
        }}
        rehypePlugins={[[rehypeHighlight, { detect: true }]]}
        remarkPlugins={[remarkGfm]}
      >
        {text}
      </ReactMarkdown>
    </div>
  )
}
