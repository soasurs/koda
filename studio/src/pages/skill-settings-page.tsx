import { useQuery } from '@tanstack/react-query'
import { ChevronRight, FileText, LoaderCircle, PackageOpen } from 'lucide-react'
import { lazy, Suspense, useState } from 'react'

import { SettingsLayout } from '@/components/settings/settings-layout'
import { Modal } from '@/components/ui/modal'
import type { Skill } from '@/gen/koda/v1/service_pb'
import { errorMessage, getSkill, kodaKeys, listSkills } from '@/lib/koda'

const MarkdownText = lazy(() => import('@/components/markdown-text'))

export function SkillSettingsPage() {
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
          <h2 className="text-lg font-semibold tracking-tight">Skills</h2>
          <p className="mt-1 max-w-2xl text-sm leading-6 text-neutral-500">
            Inspect the Agent Skills loaded from ~/.koda/skills when this Koda
            process started. Restart Koda to pick up filesystem changes.
          </p>
        </div>
      </div>

      {skillsQuery.isPending ? (
        <div className="flex h-56 items-center justify-center">
          <LoaderCircle className="size-5 animate-spin text-neutral-600" />
        </div>
      ) : skillsQuery.isError ? (
        <p className="error-box mt-6">{errorMessage(skillsQuery.error)}</p>
      ) : skillsQuery.data.length === 0 ? (
        <div className="mt-6 rounded-lg border border-dashed border-neutral-800 px-6 py-12 text-center">
          <PackageOpen className="mx-auto size-6 text-neutral-600" />
          <p className="mt-3 text-sm font-medium text-neutral-300">
            No skills loaded
          </p>
          <p className="mt-1 text-sm text-neutral-600">
            Add a skill under ~/.koda/skills and restart Koda.
          </p>
        </div>
      ) : (
        <div className="mt-6 grid gap-3 sm:grid-cols-2">
          {skillsQuery.data.map((skill) => (
            <button
              aria-label={`Open ${skill.name}`}
              className="group flex min-h-28 items-start gap-3 rounded-lg border border-neutral-800 bg-neutral-950 p-4 text-left transition hover:border-neutral-700 hover:bg-neutral-900/60"
              key={skill.name}
              onClick={() => setSelectedName(skill.name)}
              type="button"
            >
              <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-neutral-900 text-neutral-500 group-hover:text-neutral-300">
                <FileText className="size-4" aria-hidden="true" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium text-neutral-200">
                  {skill.name}
                </span>
                <span className="mt-1 line-clamp-2 block text-xs leading-5 text-neutral-600">
                  {skill.description}
                </span>
              </span>
              <ChevronRight
                className="mt-1 size-4 shrink-0 text-neutral-700 group-hover:text-neutral-400"
                aria-hidden="true"
              />
            </button>
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
                <LoaderCircle className="size-5 animate-spin text-neutral-600" />
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
  const metadata = Object.entries(skill.metadata)
  return (
    <article className="min-w-0">
      {(skill.license || skill.compatibility || metadata.length > 0) && (
        <dl className="grid gap-3 border-b border-neutral-800 pb-5 text-sm sm:grid-cols-2">
          {skill.license && <Detail label="License" value={skill.license} />}
          {skill.compatibility && (
            <Detail label="Compatibility" value={skill.compatibility} />
          )}
          {metadata.map(([key, value]) => (
            <Detail key={key} label={key} value={value} />
          ))}
        </dl>
      )}

      {skill.allowedTools.length > 0 && (
        <DetailList label="Allowed tools" values={skill.allowedTools} />
      )}

      <section className="mt-6">
        <h4 className="text-xs font-medium uppercase tracking-wider text-neutral-600">
          Instructions
        </h4>
        <div className="mt-3 min-w-0 text-sm leading-6 text-neutral-300">
          <Suspense
            fallback={
              <LoaderCircle className="size-4 animate-spin text-neutral-600" />
            }
          >
            <MarkdownText text={skill.instructions} />
          </Suspense>
        </div>
      </section>

      {skill.resources.length > 0 && (
        <DetailList label="Resources" values={skill.resources} />
      )}
    </article>
  )
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wider text-neutral-600">
        {label}
      </dt>
      <dd className="mt-1 break-words text-neutral-300">{value}</dd>
    </div>
  )
}

function DetailList({ label, values }: { label: string; values: string[] }) {
  return (
    <section className="mt-6">
      <h4 className="text-xs font-medium uppercase tracking-wider text-neutral-600">
        {label}
      </h4>
      <div className="mt-2 flex flex-wrap gap-2">
        {values.map((value) => (
          <code
            className="rounded border border-neutral-800 bg-neutral-900 px-2 py-1 text-xs text-neutral-400"
            key={value}
          >
            {value}
          </code>
        ))}
      </div>
    </section>
  )
}
