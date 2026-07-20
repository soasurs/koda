import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import MarkdownText from '@/components/markdown-text'

describe('MarkdownText', () => {
  it('renders fenced code with syntax highlighting', () => {
    const { container } = render(
      <MarkdownText text={'```ts\nconst answer = 42\n```'} />,
    )

    expect(container.querySelector('code.language-ts.hljs')).not.toBeNull()
    expect(container.querySelector('.hljs-keyword')).toHaveTextContent('const')
  })

  it('renders links with target=_blank and rel=noreferrer', () => {
    const { container } = render(
      <MarkdownText text={'[GitHub](https://github.com)'} />,
    )

    const link = container.querySelector('a')
    expect(link).not.toBeNull()
    expect(link!.getAttribute('target')).toBe('_blank')
    expect(link!.getAttribute('rel')).toBe('noreferrer')
    expect(link!.getAttribute('href')).toBe('https://github.com')
  })
})
