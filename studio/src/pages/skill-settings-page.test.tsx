import { create } from '@bufbuild/protobuf'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { I18nProvider } from '@/app/i18n'
import { SkillSchema, SkillSummarySchema } from '@/gen/koda/v1/service_pb'

const { getSkillMock, listSkillsMock } = vi.hoisted(() => ({
  getSkillMock: vi.fn(),
  listSkillsMock: vi.fn(),
}))

vi.mock('@/components/settings/settings-layout', () => ({
  SettingsLayout: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}))

vi.mock('@/lib/koda', () => ({
  errorMessage: (error: Error) => error.message,
  getSkill: getSkillMock,
  kodaKeys: {
    skills: ['skills'],
    skill: (name: string) => ['skill', name],
  },
  listSkills: listSkillsMock,
}))

import { SkillSettingsPage } from '@/pages/skill-settings-page'

describe('SkillSettingsPage', () => {
  beforeEach(() => {
    listSkillsMock.mockReset()
    getSkillMock.mockReset()
  })

  it('shows the loaded skill list and complete selected definition', async () => {
    listSkillsMock.mockResolvedValue([
      create(SkillSummarySchema, {
        name: 'review-go',
        description: 'Review Go code.',
      }),
    ])
    getSkillMock.mockResolvedValue(
      create(SkillSchema, {
        name: 'review-go',
        description: 'Review Go code.',
        license: 'MIT',
        compatibility: 'Go 1.26',
        metadata: { owner: 'koda' },
        allowedTools: ['read_file'],
        instructions: '## Checklist\n\nCheck cancellation.',
        resources: ['references/checklist.md'],
      }),
    )
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <I18nProvider>
          <SkillSettingsPage />
        </I18nProvider>
      </QueryClientProvider>,
    )

    expect(getSkillMock).not.toHaveBeenCalled()
    await userEvent.click(
      await screen.findByRole('button', {
        name: 'Open review-go',
      }),
    )
    expect(
      await screen.findByRole('heading', { name: 'review-go' }),
    ).toBeInTheDocument()
    expect(await screen.findByText('Check cancellation.')).toBeInTheDocument()
    expect(screen.getByText('MIT')).toBeInTheDocument()
    expect(screen.getByText('read_file')).toBeInTheDocument()
    expect(screen.getByText('references/checklist.md')).toBeInTheDocument()
    expect(getSkillMock).toHaveBeenCalledWith('review-go')
  })

  it('shows an empty state when no skills were loaded', async () => {
    listSkillsMock.mockResolvedValue([])
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <I18nProvider>
          <SkillSettingsPage />
        </I18nProvider>
      </QueryClientProvider>,
    )

    expect(await screen.findByText('No skills loaded')).toBeInTheDocument()
    expect(getSkillMock).not.toHaveBeenCalled()
  })
})
