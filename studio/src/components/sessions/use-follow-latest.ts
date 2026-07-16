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
    if (!container) return
    const newHeight = container.scrollHeight
    const grew = newHeight > prevScrollHeightRef.current
    prevScrollHeightRef.current = newHeight
    if (shouldFollowLatestRef.current && grew) {
      container.scrollTop = newHeight
    }
  }, [content, resetKey])

  const onScroll = useCallback((event: UIEvent<T>) => {
    const container = event.currentTarget
    const distanceFromBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight
    const shouldFollowLatest = distanceFromBottom <= bottomThreshold
    if (shouldFollowLatest && !shouldFollowLatestRef.current) {
      prevScrollHeightRef.current = 0
    }
    shouldFollowLatestRef.current = shouldFollowLatest
  }, [])

  return { containerRef, onScroll }
}
