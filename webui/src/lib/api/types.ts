// Wire types of docs/openapi/api-v0.yaml. Hand-written like libs/client: keep
// them in step with the spec's schemas.

export interface Info {
    version: string
    buildTime: string
    commit: string
    goVersion: string
}

export interface Me {
    userId: string
    subject: string
    tenantId: string
    tenant: string
}

export interface Note {
    id: string
    createdBy: string
    title: string
    body: string
    createdAt: string
    // Ids of the tags the note carries, ascending.
    tagIds: number[]
}

export interface NoteInput {
    title: string
    body?: string
}

// A tag with its subtree. parentId is 0 for a tag at the root; children are in
// the order the users gave them.
export interface Tag {
    id: number
    parentId: number
    name: string
    children: Tag[]
}

// The body of POST /tags: parentId 0 (or absent) is the root; afterId is the
// sibling to place it after, 0 (or absent) first.
export interface TagInput {
    name: string
    parentId?: number
    afterId?: number
}

// The body of PATCH /tags/{tagId}: an absent field is left as it is.
export interface TagPatch {
    name?: string
    parentId?: number
    afterId?: number
}

// A job the server defines, with the caller's tenant's schedules of it.
export interface Job {
    name: string
    title: string
    description: string
    schedules: JobSchedule[]
}

// A cron schedule of a job; cron is the stored 6-field form (seconds first).
export interface JobSchedule {
    id: string
    cron: string
    enabled: boolean
    // Absent while the schedule is disabled.
    nextRunAt?: string
    createdAt: string
    updatedAt: string
}

// The body of POST /jobs/{jobName}/schedules.
export interface JobScheduleInput {
    cron: string
    enabled?: boolean
}

// The body of PATCH /jobs/{jobName}/schedules/{scheduleId}: absent fields stay.
export interface JobSchedulePatch {
    cron?: string
    enabled?: boolean
}

export type JobRunStatus =
    'waiting' | 'running' | 'complete' | 'failed' | 'panicked' | 'canceled' | 'cancel_error'

// The latest progress of a running run: total is 0 for a step of unknown size.
export interface JobProgress {
    done: number
    total: number
    stage?: string
}

export interface JobRun {
    id: string
    job: string
    status: JobRunStatus
    queuedAt: string
    startedAt?: string
    endedAt?: string
    // Present only while the run is running.
    progress?: JobProgress
}
