import { useMemo } from 'react'

export function useTokenFormatter(locale: string) {
  return useMemo(
    () =>
      new Intl.NumberFormat(locale, {
        maximumFractionDigits: 1,
        notation: 'compact',
      }),
    [locale],
  )
}
