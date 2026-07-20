import { useQuery } from '@tanstack/react-query'
import { ChevronRight, FileText, LoaderCircle, PackageOpen } from 'lucide-react'
import { lazy, Suspense, useState } from 'react'

import { useI18n } from '@/app/i18n'
import { Button } from '@/components/ui/button'
import { SettingsLayout } from '@/components/settings/settings-layout'
import { Modal } from '@/components/ui/modal'
import type { Skill } from '@/gen/koda/v1/service_pb'
import { errorMessage, getSkill, kodaKeys, listSkills } from '@/lib/koda'

const MarkdownText = lazy(() => import('@/components/markdown-text'))

export function SkillSettingsPage() {
  const { t } = useI18n()
  const [selectedName, setSelectedName] = useState<string>()
  const skillsQuery = useQuery({
    queryKey: kodaKeys.skills,
    queryFn: listSkills,
  })
  const skillQuery = useQuery({
    queryKey: kodaKeys.skill(selectedName ?? ''),
    queryFn: () => getSkill(selectedName ?? ''),
    enabled: Boolean(selectedName),
  })
  const selectedSummary = skillsQuery.data?.find(
    (skill) => skill.name === selectedName,
  )

  return (
    <SettingsLayout active="skills">
      <div className="flex items-start justify-between gap-5">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">
            {t('settings.skills.title')}
          </h2>
          <p className="mt-1 max-w-2xl text-sm leading-6 text-muted-foreground">
            {t('settings.skills.description')}
          </p>
        </div>
      </div>

      {skillsQuery.isPending ? (
        <div className="flex h-56 items-center justify-center">
          <LoaderCircle className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : skillsQuery.isError ? (
        <p className="error-box mt-6">{errorMessage(skillsQuery.error)}</p>
      ) : skillsQuery.data.length === 0 ? (
        <div className="mt-6 rounded-lg border border-dashed border-border px-6 py-12 text-center">
          <PackageOpen className="mx-auto size-6 text-muted-foreground" />
          <p className="mt-3 text-sm font-medium text-foreground">
            {t('settings.skills.empty.title')}
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('settings.skills.empty.body')}
          </p>
        </div>
      ) : (
        <div className="mt-6 grid gap-3 sm:grid-cols-2">
          {skillsQuery.data.map((skill) => (
            <Button
              aria-label={t('settings.skills.card.openAria', {
                name: skill.name,
              })}
              className="group flex h-auto min-h-28 items-start gap-3 rounded-lg border border-border bg-background p-4 text-left hover:border-border/80 hover:bg-accent"
              key={skill.name}
              onClick={() => setSelectedName(skill.name)}
              variant="ghost"
            >
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground group-hover:text-foreground">
                <FileText className="size-4" aria-hidden="true" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium text-foreground">
                  {skill.name}
                </span>
                <span className="mt-1 line-clamp-2 block text-xs leading-5 text-muted-foreground">
                  {skill.description}
                </span>
              </span>
              <ChevronRight
                className="mt-1 size-4 shrink-0 text-muted-foreground group-hover:text-foreground"
                aria-hidden="true"
              />
            </Button>
          ))}
        </div>
      )}

      {selectedName && (
        <Modal
          description={selectedSummary?.description}
          onClose={() => setSelectedName(undefined)}
          title={selectedName}
          wide
        >
          <div className="min-h-40 p-5 sm:p-6">
            {skillQuery.isPending ? (
              <div className="flex min-h-40 items-center justify-center">
                <LoaderCircle className="size-5 animate-spin text-muted-foreground" />
              </div>
            ) : skillQuery.isError ? (
              <p className="error-box">{errorMessage(skillQuery.error)}</p>
            ) : skillQuery.data ? (
              <SkillDetails skill={skillQuery.data} />
            ) : null}
          </div>
        </Modal>
      )}
    </SettingsLayout>
  )
}

function SkillDetails({ skill }: { skill: Skill }) {
  const { t } = useI18n()
  const metadata = Object.entries(skill.metadata)
  return (
    <article className="min-w-0">
      {(skill.license || skill.compatibility || metadata.length > 0) && (
        <dl className="grid gap-3 border-b border-border pb-5 text-sm sm:grid-cols-2">
          {skill.license && (
            <Detail
              label={t('settings.skills.details.license')}
              value={skill.license}
            />
          )}
          {skill.compatibility && (
            <Detail
              label={t('settings.skills.details.compatibility')}
              value={skill.compatibility}
            />
          )}
          {metadata.map(([key, value]) => (
            <Detail key={key} label={key} value={value} />
          ))}
        </dl>
      )}

      {skill.allowedTools.length > 0 && (
        <DetailList
          label={t('settings.skills.details.allowedTools')}
          values={skill.allowedTools}
        />
      )}

      <section className="mt-6">
        <h4 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          {t('settings.skills.details.instructions')}
        </h4>
        <div className="mt-3 min-w-0 text-sm leading-6 text-foreground">
          <Suspense
            fallback={
              <LoaderCircle className="size-4 animate-spin text-muted-foreground" />
            }
          >
            <MarkdownText text={skill.instructions} />
          </Suspense>
        </div>
      </section>

      {skill.resources.length > 0 && (
        <DetailList
          label={t('settings.skills.details.resources')}
          values={skill.resources}
        />
      )}
    </article>
  )
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {label}
      </dt>
      <dd className="mt-1 break-words text-foreground">{value}</dd>
    </div>
  )
}

function DetailList({ label, values }: { label: string; values: string[] }) {
  return (
    <section className="mt-6">
      <h4 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {label}
      </h4>
      <div className="mt-2 flex flex-wrap gap-2">
        {values.map((value) => (
          <code
            className="rounded border border-border bg-muted px-2 py-1 text-xs text-muted-foreground"
            key={value}
          >
            {value}
          </code>
        ))}
      </div>
    </section>
  )
}
