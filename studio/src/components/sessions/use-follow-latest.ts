import { useCallback, useLayoutEffect, useRef } from 'react'
import type { UIEvent } from 'react'

const bottomThreshold = 48

export function useFollowLatest<T extends HTMLElement>(
  content: unknown,
  resetKey: string,
) {
  const containerRef = useRef<T>(null)
  const shouldFollowLatestRef = useRef(true)

  useLayoutEffect(() => {
    shouldFollowLatestRef.current = true
  }, [resetKey])

  useLayoutEffect(() => {
    const container = containerRef.current
    if (container && shouldFollowLatestRef.current) {
      container.scrollTop = container.scrollHeight
    }
  }, [content, resetKey])

  const onScroll = useCallback((event: UIEvent<T>) => {
    const container = event.currentTarget
    const distanceFromBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight
    shouldFollowLatestRef.current = distanceFromBottom <= bottomThreshold
  }, [])

  return { containerRef, onScroll }
}
