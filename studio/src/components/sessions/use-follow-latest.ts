import { useCallback, useLayoutEffect, useRef } from 'react'
import type { UIEvent } from 'react'

const bottomThreshold = 48

export function useFollowLatest<T extends HTMLElement>(
  content: unknown,
  resetKey: string,
) {
  const containerRef = useRef<T>(null)
  const shouldFollowLatestRef = useRef(true)
  const prevScrollHeightRef = useRef(0)

  useLayoutEffect(() => {
    shouldFollowLatestRef.current = true
    prevScrollHeightRef.current = 0
  }, [resetKey])

  useLayoutEffect(() => {
    const container = containerRef.current
    if (!container || !shouldFollowLatestRef.current) return
    const newHeight = container.scrollHeight
    if (newHeight <= prevScrollHeightRef.current) return
    prevScrollHeightRef.current = newHeight
    container.scrollTop = container.scrollHeight
  }, [content, resetKey])

  const onScroll = useCallback((event: UIEvent<T>) => {
    const container = event.currentTarget
    const distanceFromBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight
    shouldFollowLatestRef.current = distanceFromBottom <= bottomThreshold
  }, [])

  return { containerRef, onScroll }
}
