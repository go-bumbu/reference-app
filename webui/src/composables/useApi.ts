import { computed, type MaybeRefOrGetter, toValue } from 'vue'
import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/vue-query'
import {
    addNoteTag,
    createNote,
    createTag,
    deleteNote,
    deleteTag,
    getInfo,
    getMe,
    getNote,
    listNotes,
    listTags,
    removeNoteTag,
    updateNote,
    updateTag
} from '@/lib/api/api'
import type { Note, NoteInput, TagInput, TagPatch } from '@/lib/api/types'

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
    tags: ['tags'] as const
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
