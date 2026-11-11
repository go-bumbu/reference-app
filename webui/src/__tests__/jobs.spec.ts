import { describe, expect, it } from 'vitest'
import {
    hasActiveRuns,
    isActiveStatus,
    jobRows,
    latestRuns,
    progressLabel,
    runDuration,
    scheduleSummary,
    statusLabel,
    statusSeverity
} from '@/lib/jobs'
import type { Job, JobRun, JobSchedule } from '@/lib/api/types'

const run = (over: Partial<JobRun>): JobRun => ({
    id: 'r',
    job: 'scan-notes',
    status: 'complete',
    queuedAt: '2026-11-10T10:00:00Z',
    ...over
})

const schedule = (over: Partial<JobSchedule>): JobSchedule => ({
    id: 's',
    cron: '0 0 0 * * *',
    enabled: true,
    createdAt: '2026-11-10T10:00:00Z',
    updatedAt: '2026-11-10T10:00:00Z',
    ...over
})

describe('jobs helpers', () => {
    it('knows which runs are still going', () => {
        expect(isActiveStatus('waiting')).toBe(true)
        expect(isActiveStatus('running')).toBe(true)
        expect(isActiveStatus('complete')).toBe(false)
        expect(isActiveStatus(null)).toBe(false)
        expect(hasActiveRuns([run({ status: 'failed' }), run({ status: 'running' })])).toBe(true)
        expect(hasActiveRuns(undefined)).toBe(false)
    })

    it('labels and colours a status', () => {
        expect(statusLabel('waiting')).toBe('queued')
        expect(statusLabel('complete')).toBe('complete')
        expect(statusSeverity('complete')).toBe('success')
        expect(statusSeverity('panicked')).toBe('danger')
        expect(statusSeverity('running')).toBe('info')
    })

    it('shows a percentage only for a step of known size', () => {
        expect(progressLabel('running', { done: 1, total: 4 })).toBe('25% complete')
        expect(progressLabel('running', { done: 3, total: 0 })).toBe('Running')
        expect(progressLabel('waiting', undefined)).toBe('Queued')
    })

    it('summarises the schedules of a job', () => {
        expect(scheduleSummary([])).toBe('Not scheduled')
        expect(scheduleSummary([schedule({})])).toBe('Daily')
        expect(scheduleSummary([schedule({ cron: '0 30 4 * * *', enabled: false })])).toBe(
            '0 30 4 * * * (paused)'
        )
        expect(scheduleSummary([schedule({}), schedule({ id: 't' })])).toBe('2 schedules')
    })

    it('finds the latest run of each job', () => {
        const older = run({ id: 'a', queuedAt: '2026-11-10T09:00:00Z' })
        const newer = run({ id: 'b', queuedAt: '2026-11-10T11:00:00Z' })
        expect(latestRuns([newer, older]).get('scan-notes')?.id).toBe('b')

        const job: Job = { name: 'scan-notes', title: 'Scan notes', description: '', schedules: [] }
        const other: Job = { ...job, name: 'other' }
        const rows = jobRows([job, other], [older, newer])
        expect(rows[0].lastRun?.id).toBe('b')
        expect(rows[1].lastRun).toBeNull()
    })

    it('measures how long a run took', () => {
        expect(runDuration({})).toBe('-')
        const startedAt = '2026-11-10T10:00:00Z'
        expect(runDuration({ startedAt, endedAt: '2026-11-10T10:00:00.250Z' })).toBe('250ms')
        expect(runDuration({ startedAt, endedAt: '2026-11-10T10:00:42Z' })).toBe('42s')
        expect(runDuration({ startedAt, endedAt: '2026-11-10T10:02:05Z' })).toBe('2m 5s')
    })
})
