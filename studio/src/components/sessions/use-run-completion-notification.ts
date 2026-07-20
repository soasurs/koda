import { useEffect, useRef } from 'react'

import { usePreferences } from '@/app/preferences-context-value'

/**
 * Shows a browser notification when a run completes while the page is not
 * visible (user is on another tab). The user can enable or disable this in
 * General Settings.
 */
export function useRunCompletionNotification(isRunning: boolean, body: string) {
  const { notifyOnCompletion } = usePreferences()
  const wasRunningRef = useRef(false)
  const permissionRef = useRef<NotificationPermission | 'unrequested'>(
    'unrequested',
  )

  // Request permission when the preference is enabled and we haven't asked yet.
  useEffect(() => {
    if (
      notifyOnCompletion &&
      permissionRef.current === 'unrequested' &&
      'Notification' in window &&
      Notification.permission === 'default'
    ) {
      Notification.requestPermission().then((result) => {
        permissionRef.current = result
      })
    }
    if ('Notification' in window && Notification.permission !== 'default') {
      permissionRef.current = Notification.permission
    }
  }, [notifyOnCompletion])

  useEffect(() => {
    // Detect transition from running to not running
    if (wasRunningRef.current && !isRunning) {
      wasRunningRef.current = false

      if (
        !notifyOnCompletion ||
        document.visibilityState === 'visible' ||
        !('Notification' in window) ||
        Notification.permission !== 'granted'
      ) {
        return
      }

      const notification = new Notification('Koda', {
        body,
        tag: 'koda-run-completion',
        icon: '/favicon.svg',
        requireInteraction: true,
      })
      notification.onclick = () => {
        window.focus()
        notification.close()
      }
    }

    if (isRunning) {
      wasRunningRef.current = true
    }
  }, [isRunning, notifyOnCompletion, body])
}
