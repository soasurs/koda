import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'

import { I18nProvider } from '@/app/i18n'
import { queryClient } from '@/app/query-client'
import { router } from '@/app/router'
import { PreferencesProvider } from '@/app/preferences-provider'
import { ThemeProvider } from '@/app/theme'
import '@/styles.css'

const rootElement = document.getElementById('root')

if (!rootElement) {
  throw new Error('root element not found')
}

createRoot(rootElement).render(
  <StrictMode>
    <ThemeProvider>
      <I18nProvider>
        <PreferencesProvider>
          <QueryClientProvider client={queryClient}>
            <RouterProvider router={router} />
          </QueryClientProvider>
        </PreferencesProvider>
      </I18nProvider>
    </ThemeProvider>
  </StrictMode>,
)
