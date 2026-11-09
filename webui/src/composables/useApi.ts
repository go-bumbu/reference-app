import { computed, type MaybeRefOrGetter, toValue } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import {
    createNote,
    deleteNote,
    getInfo,
    getMe,
    getNote,
    listNotes,
    updateNote
} from '@/lib/api/api'
import type { NoteInput } from '@/lib/api/types'

// Query keys in one place, so a mutation invalidates exactly what it changed.
export const queryKeys = {
    info: ['info'] as const,
    me: ['me'] as const,
    notes: ['notes'] as const,
    note: (id: string) => ['notes', id] as const
}

export function useInfo() {
    return useQuery({ queryKey: queryKeys.info, queryFn: getInfo, staleTime: Infinity })
}

export function useMe() {
    return useQuery({ queryKey: queryKeys.me, queryFn: getMe, staleTime: Infinity })
}

export function useNotes() {
    return useQuery({ queryKey: queryKeys.notes, queryFn: listNotes })
}

export function useNote(id: MaybeRefOrGetter<string>) {
    return useQuery({
        queryKey: computed(() => queryKeys.note(toValue(id))),
        queryFn: () => getNote(toValue(id)),
        // a 404 will not turn into a 200 by asking again
        retry: false
    })
}

export function useCreateNote() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: (input: NoteInput) => createNote(input),
        onSuccess: (note) => {
            qc.setQueryData(queryKeys.note(note.id), note)
            return qc.invalidateQueries({ queryKey: queryKeys.notes, exact: true })
        }
    })
}

export function useUpdateNote() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: ({ id, input }: { id: string; input: NoteInput }) => updateNote(id, input),
        onSuccess: (note) => {
            qc.setQueryData(queryKeys.note(note.id), note)
            return qc.invalidateQueries({ queryKey: queryKeys.notes, exact: true })
        }
    })
}

export function useDeleteNote() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: (id: string) => deleteNote(id),
        onSuccess: (_data, id) => {
            qc.removeQueries({ queryKey: queryKeys.note(id), exact: true })
            return qc.invalidateQueries({ queryKey: queryKeys.notes, exact: true })
        }
    })
}
