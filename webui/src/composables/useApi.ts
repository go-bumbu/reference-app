import { computed, type MaybeRefOrGetter, toValue } from 'vue'
import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/vue-query'
import {
    addNoteTag,
    cancelJobRun,
    createJobSchedule,
    createNote,
    createTag,
    deleteJobSchedule,
    deleteNote,
    deleteTag,
    getInfo,
    getJobRunLog,
    getMe,
    getNote,
    listJobRuns,
    listJobs,
    listNotes,
    listTags,
    removeNoteTag,
    runJob,
    updateJobSchedule,
    updateNote,
    updateTag
} from '@/lib/api/api'
import type {
    JobScheduleInput,
    JobSchedulePatch,
    Note,
    NoteInput,
    TagInput,
    TagPatch
} from '@/lib/api/types'
import { hasActiveRuns } from '@/lib/jobs'

// How often the runs are refetched while one is active, and a run's open log.
const JOB_RUNS_POLL_MS = 1000
const JOB_LOG_POLL_MS = 2000

// Query keys in one place, so a mutation invalidates exactly what it changed.
// Every notes key starts with 'notes': the lists (one per tag filter) under
// 'list', each note under 'detail'.
export const queryKeys = {
    info: ['info'] as const,
    me: ['me'] as const,
    notes: ['notes'] as const,
    noteLists: ['notes', 'list'] as const,
    noteList: (tagId: number | null) => ['notes', 'list', tagId ?? 'all'] as const,
    note: (id: string) => ['notes', 'detail', id] as const,
    tags: ['tags'] as const,
    // Every jobs key starts with 'jobs': the job list itself, the runs under
    // 'runs', and each run's log under the run.
    jobs: ['jobs'] as const,
    jobRuns: ['jobs', 'runs'] as const,
    jobRunLog: (id: string) => ['jobs', 'runs', id, 'log'] as const
}

export function useInfo() {
    return useQuery({ queryKey: queryKeys.info, queryFn: getInfo, staleTime: Infinity })
}

export function useMe() {
    return useQuery({ queryKey: queryKeys.me, queryFn: getMe, staleTime: Infinity })
}

// The notes of the tenant, or of a tag (null for every note); the server
// narrows the list, the subtree of the tag included.
export function useNotes(tagId: MaybeRefOrGetter<number | null> = null) {
    return useQuery({
        queryKey: computed(() => queryKeys.noteList(toValue(tagId))),
        queryFn: () => listNotes(toValue(tagId) ?? undefined)
    })
}

export function useNote(id: MaybeRefOrGetter<string>) {
    return useQuery({
        queryKey: computed(() => queryKeys.note(toValue(id))),
        queryFn: () => getNote(toValue(id)),
        // a 404 will not turn into a 200 by asking again
        retry: false
    })
}

// A note the server returned is the note's own query from then on, and every
// list may hold it: the lists are refetched.
function noteChanged(qc: QueryClient, note: Note) {
    qc.setQueryData(queryKeys.note(note.id), note)
    return qc.invalidateQueries({ queryKey: queryKeys.noteLists })
}

export function useCreateNote() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: (input: NoteInput) => createNote(input),
        onSuccess: (note) => noteChanged(qc, note)
    })
}

export function useUpdateNote() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: ({ id, input }: { id: string; input: NoteInput }) => updateNote(id, input),
        onSuccess: (note) => noteChanged(qc, note)
    })
}

export function useDeleteNote() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: (id: string) => deleteNote(id),
        onSuccess: (_data, id) => {
            qc.removeQueries({ queryKey: queryKeys.note(id), exact: true })
            return qc.invalidateQueries({ queryKey: queryKeys.noteLists })
        }
    })
}

// Gives a note a tag (`add`) or takes it off. Both are idempotent on the
// server, so a repeated drop of a note on the same tag is harmless.
export function useSetNoteTag() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: ({ noteId, tagId, add }: { noteId: string; tagId: number; add: boolean }) =>
            add ? addNoteTag(noteId, tagId) : removeNoteTag(noteId, tagId),
        onSuccess: (note) => noteChanged(qc, note)
    })
}

export function useTags() {
    return useQuery({ queryKey: queryKeys.tags, queryFn: listTags })
}

// Creating, renaming and moving a tag change the tree alone: a move changes
// which notes a tag's list holds, so the lists are refetched too.
export function useCreateTag() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: (input: TagInput) => createTag(input),
        onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.tags })
    })
}

export function useUpdateTag() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: ({ id, patch }: { id: number; patch: TagPatch }) => updateTag(id, patch),
        onSuccess: (_tag, { patch }) => {
            const moved = patch.parentId !== undefined
            return Promise.all([
                qc.invalidateQueries({ queryKey: queryKeys.tags }),
                moved ? qc.invalidateQueries({ queryKey: queryKeys.noteLists }) : undefined
            ])
        }
    })
}

// Deleting a tag takes it and its subtree off every note: the tree, the lists
// and the open notes are all refetched.
export function useDeleteTag() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: (id: number) => deleteTag(id),
        onSuccess: () =>
            Promise.all([
                qc.invalidateQueries({ queryKey: queryKeys.tags }),
                qc.invalidateQueries({ queryKey: queryKeys.notes })
            ])
    })
}

// The jobs the server defines, with the tenant's schedules of each. A schedule
// change invalidates it.
export function useJobs() {
    return useQuery({ queryKey: queryKeys.jobs, queryFn: listJobs })
}

// The tenant's runs, polled while one of them is waiting or running.
export function useJobRuns() {
    return useQuery({
        queryKey: queryKeys.jobRuns,
        queryFn: ({ signal }) => listJobRuns(signal),
        refetchInterval: (query) => (hasActiveRuns(query.state.data) ? JOB_RUNS_POLL_MS : false),
        refetchIntervalInBackground: false
    })
}

// A run's log, polled while `enabled` (the log dialog is open).
export function useJobRunLog(id: MaybeRefOrGetter<string>, enabled: MaybeRefOrGetter<boolean>) {
    return useQuery({
        queryKey: computed(() => queryKeys.jobRunLog(toValue(id))),
        queryFn: () => getJobRunLog(toValue(id)),
        enabled: computed(() => toValue(enabled) && toValue(id) !== ''),
        staleTime: 0,
        refetchInterval: JOB_LOG_POLL_MS
    })
}

// Starts a run; the runs are refetched whether it was queued or reused. The
// caller reads `reused` to tell the user nothing new was queued.
export function useRunJob() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: (name: string) => runJob(name),
        onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.jobRuns })
    })
}

export function useCancelJobRun() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: (id: string) => cancelJobRun(id),
        onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.jobRuns })
    })
}

// A schedule lives on its job in GET /jobs, so every schedule change refetches
// the jobs (exact: the runs share the 'jobs' prefix and did not change).
function schedulesChanged(qc: QueryClient) {
    return qc.invalidateQueries({ queryKey: queryKeys.jobs, exact: true })
}

export function useCreateJobSchedule() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: ({ job, input }: { job: string; input: JobScheduleInput }) =>
            createJobSchedule(job, input),
        onSuccess: () => schedulesChanged(qc)
    })
}

export function useUpdateJobSchedule() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: ({ job, id, patch }: { job: string; id: string; patch: JobSchedulePatch }) =>
            updateJobSchedule(job, id, patch),
        onSuccess: () => schedulesChanged(qc)
    })
}

export function useDeleteJobSchedule() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: ({ job, id }: { job: string; id: string }) => deleteJobSchedule(job, id),
        onSuccess: () => schedulesChanged(qc)
    })
}
