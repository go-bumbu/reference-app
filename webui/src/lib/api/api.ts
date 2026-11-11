import { apiClient } from './client'
import type {
    Info,
    Job,
    JobRun,
    JobSchedule,
    JobScheduleInput,
    JobSchedulePatch,
    Me,
    Note,
    NoteInput,
    Tag,
    TagInput,
    TagPatch
} from './types'

export async function getInfo(): Promise<Info> {
    return (await apiClient.get<Info>('/info')).data
}

export async function getMe(): Promise<Me> {
    return (await apiClient.get<Me>('/me')).data
}

// The notes of the tenant, or, given a tag, those that carry it or a tag below it.
export async function listNotes(tagId?: number): Promise<Note[]> {
    const params = tagId ? { tagId } : undefined
    return (await apiClient.get<{ items: Note[] }>('/notes', { params })).data.items
}

export async function getNote(id: string): Promise<Note> {
    return (await apiClient.get<Note>(`/notes/${encodeURIComponent(id)}`)).data
}

export async function createNote(input: NoteInput): Promise<Note> {
    return (await apiClient.post<Note>('/notes', input)).data
}

export async function updateNote(id: string, input: NoteInput): Promise<Note> {
    return (await apiClient.put<Note>(`/notes/${encodeURIComponent(id)}`, input)).data
}

export async function deleteNote(id: string): Promise<void> {
    await apiClient.delete(`/notes/${encodeURIComponent(id)}`)
}

export async function addNoteTag(noteId: string, tagId: number): Promise<Note> {
    return (await apiClient.put<Note>(`/notes/${encodeURIComponent(noteId)}/tags/${tagId}`)).data
}

export async function removeNoteTag(noteId: string, tagId: number): Promise<Note> {
    return (await apiClient.delete<Note>(`/notes/${encodeURIComponent(noteId)}/tags/${tagId}`)).data
}

export async function listTags(): Promise<Tag[]> {
    return (await apiClient.get<{ items: Tag[] }>('/tags')).data.items
}

export async function createTag(input: TagInput): Promise<Tag> {
    return (await apiClient.post<Tag>('/tags', input)).data
}

export async function updateTag(id: number, patch: TagPatch): Promise<Tag> {
    return (await apiClient.patch<Tag>(`/tags/${id}`, patch)).data
}

export async function deleteTag(id: number): Promise<void> {
    await apiClient.delete(`/tags/${id}`)
}

export async function listJobs(): Promise<Job[]> {
    return (await apiClient.get<{ items: Job[] }>('/jobs')).data.items
}

// Queues a run of the job for the tenant. reused is true when the tenant
// already had one waiting or running: the server answers 200 with it, not 202.
export async function runJob(name: string): Promise<{ run: JobRun; reused: boolean }> {
    const res = await apiClient.post<JobRun>(`/jobs/${encodeURIComponent(name)}/runs`)
    return { run: res.data, reused: res.status === 200 }
}

export async function listJobRuns(signal?: AbortSignal): Promise<JobRun[]> {
    return (await apiClient.get<{ items: JobRun[] }>('/job-runs', { signal })).data.items
}

export async function getJobRun(id: string): Promise<JobRun> {
    return (await apiClient.get<JobRun>(`/job-runs/${encodeURIComponent(id)}`)).data
}

export async function cancelJobRun(id: string): Promise<void> {
    await apiClient.post(`/job-runs/${encodeURIComponent(id)}/cancel`)
}

// The last lines of a run's log as plain text (the server caps tail at 1000).
export async function getJobRunLog(id: string, tail?: number): Promise<string> {
    const params = tail ? { tail } : undefined
    const res = await apiClient.get<string>(`/job-runs/${encodeURIComponent(id)}/log`, {
        params,
        responseType: 'text'
    })
    return res.data ?? ''
}

function schedulesPath(jobName: string) {
    return `/jobs/${encodeURIComponent(jobName)}/schedules`
}

export async function createJobSchedule(
    jobName: string,
    input: JobScheduleInput
): Promise<JobSchedule> {
    return (await apiClient.post<JobSchedule>(schedulesPath(jobName), input)).data
}

export async function updateJobSchedule(
    jobName: string,
    id: string,
    patch: JobSchedulePatch
): Promise<JobSchedule> {
    return (
        await apiClient.patch<JobSchedule>(
            `${schedulesPath(jobName)}/${encodeURIComponent(id)}`,
            patch
        )
    ).data
}

export async function deleteJobSchedule(jobName: string, id: string): Promise<void> {
    await apiClient.delete(`${schedulesPath(jobName)}/${encodeURIComponent(id)}`)
}
