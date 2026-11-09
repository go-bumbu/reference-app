import { apiClient } from './client'
import type { Info, Me, Note, NoteInput } from './types'

export async function getInfo(): Promise<Info> {
    return (await apiClient.get<Info>('/info')).data
}

export async function getMe(): Promise<Me> {
    return (await apiClient.get<Me>('/me')).data
}

export async function listNotes(): Promise<Note[]> {
    return (await apiClient.get<{ items: Note[] }>('/notes')).data.items
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
