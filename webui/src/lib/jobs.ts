// Pure helpers of the jobs page, after aether's useTasks: how a run's status
// reads, what the Run button says while a run is going, and which run is a
// job's latest.
import type { Job, JobProgress, JobRun, JobRunStatus, JobSchedule } from '@/lib/api/types'

export type StatusSeverity = 'info' | 'warn' | 'success' | 'danger' | 'secondary' | 'contrast'

// The schedules the dialog offers in one click, in the 6-field form the server
// stores (seconds first), so a stored schedule finds its preset again. Quartz
// numbers the days of the week from Sunday = 1.
export const SCHEDULE_PRESETS = [
    { label: 'Daily', cron: '0 0 0 * * *' },
    { label: 'Weekly (Sunday)', cron: '0 0 0 * * 1' },
    { label: 'Monthly', cron: '0 0 0 1 * *' }
] as const

/** Whether the run still has to end: waiting or running. */
export function isActiveStatus(status: JobRunStatus | string | null | undefined): boolean {
    return status === 'waiting' || status === 'running'
}

export function hasActiveRuns(runs: JobRun[] | undefined): boolean {
    return (runs ?? []).some((r) => isActiveStatus(r.status))
}

export function statusSeverity(status: JobRunStatus | string): StatusSeverity {
    switch (status) {
        case 'complete':
            return 'success'
        case 'failed':
        case 'panicked':
        case 'cancel_error':
            return 'danger'
        case 'waiting':
            return 'warn'
        case 'running':
            return 'info'
        case 'canceled':
            return 'contrast'
        default:
            return 'secondary'
    }
}

/** The status as a person reads it: a waiting run is "queued". */
export function statusLabel(status: JobRunStatus | string): string {
    if (status === 'waiting') return 'queued'
    if (status === 'cancel_error') return 'stopping'
    return status
}

/**
 * The Run button's text while a run is going: a percentage when the step has
 * a known size, else the state. The API carries done/total; the percentage is
 * derived here.
 */
export function progressLabel(
    status: JobRunStatus | string | null | undefined,
    progress: JobProgress | null | undefined
): string {
    if (progress && progress.total > 0) {
        const pct = Math.min(100, Math.round((progress.done / progress.total) * 100))
        return `${pct}% complete`
    }
    return status === 'waiting' ? 'Queued' : 'Running'
}

/** A schedule's name: its preset's label, else the cron expression. */
export function scheduleLabel(s: Pick<JobSchedule, 'cron'>): string {
    return SCHEDULE_PRESETS.find((p) => p.cron === s.cron)?.label ?? s.cron
}

/** The Jobs table's schedule column: none, the one schedule, or how many. */
export function scheduleSummary(schedules: JobSchedule[]): string {
    if (schedules.length === 0) return 'Not scheduled'
    if (schedules.length === 1) {
        const s = schedules[0]
        return s.enabled ? scheduleLabel(s) : `${scheduleLabel(s)} (paused)`
    }
    return `${schedules.length} schedules`
}

/** The latest run of each job, by name: the one queued last. */
export function latestRuns(runs: JobRun[]): Map<string, JobRun> {
    const out = new Map<string, JobRun>()
    for (const r of runs) {
        const seen = out.get(r.job)
        if (!seen || Date.parse(r.queuedAt) > Date.parse(seen.queuedAt)) out.set(r.job, r)
    }
    return out
}

export interface JobRow extends Job {
    lastRun: JobRun | null
}

/** The jobs with their latest run, for the Jobs table. */
export function jobRows(jobs: Job[], runs: JobRun[]): JobRow[] {
    const latest = latestRuns(runs)
    return jobs.map((j) => ({ ...j, lastRun: latest.get(j.name) ?? null }))
}

/** How long a run took, "-" until it ended. */
export function runDuration(r: Pick<JobRun, 'startedAt' | 'endedAt'>): string {
    if (!r.startedAt || !r.endedAt) return '-'
    const ms = Date.parse(r.endedAt) - Date.parse(r.startedAt)
    if (!(ms >= 0)) return '-'
    if (ms < 1000) return `${ms}ms`
    const sec = Math.floor(ms / 1000)
    if (sec < 60) return `${sec}s`
    const m = Math.floor(sec / 60)
    const s = sec % 60
    return s ? `${m}m ${s}s` : `${m}m`
}
