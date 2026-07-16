import { fireEvent, render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { useFollowLatest } from '@/components/sessions/use-follow-latest'

function ScrollContainer({ content, sessionId = 'session-1' }: Props) {
  const { containerRef, onScroll } = useFollowLatest<HTMLDivElement>(
    content,
    sessionId,
  )

  return <div data-testid="scroller" onScroll={onScroll} ref={containerRef} />
}

interface Props {
  content: string
  sessionId?: string
}

function setDimensions(element: HTMLElement, scrollHeight = 1_000) {
  Object.defineProperties(element, {
    clientHeight: { configurable: true, value: 400 },
    scrollHeight: { configurable: true, value: scrollHeight },
  })
}

describe('useFollowLatest', () => {
  it('follows new content while the user remains near the bottom', () => {
    const { container, rerender } = render(<ScrollContainer content="first" />)
    const scroller = container.firstElementChild as HTMLElement
    setDimensions(scroller)

    rerender(<ScrollContainer content="second" />)

    expect(scroller.scrollTop).toBe(1_000)
  })

  it('stops following when the user scrolls up and resumes at the bottom', () => {
    const { container, rerender } = render(<ScrollContainer content="first" />)
    const scroller = container.firstElementChild as HTMLElement
    setDimensions(scroller)

    scroller.scrollTop = 300
    fireEvent.scroll(scroller)
    rerender(<ScrollContainer content="second" />)
    expect(scroller.scrollTop).toBe(300)

    scroller.scrollTop = 570
    fireEvent.scroll(scroller)
    rerender(<ScrollContainer content="third" />)
    expect(scroller.scrollTop).toBe(1_000)
  })

  it('follows growth after content shrinks below an earlier height', () => {
    const { container, rerender } = render(<ScrollContainer content="first" />)
    const scroller = container.firstElementChild as HTMLElement

    setDimensions(scroller, 1_000)
    rerender(<ScrollContainer content="streaming" />)
    expect(scroller.scrollTop).toBe(1_000)

    setDimensions(scroller, 700)
    rerender(<ScrollContainer content="settled" />)

    setDimensions(scroller, 800)
    rerender(<ScrollContainer content="next message" />)
    expect(scroller.scrollTop).toBe(800)
  })
})
