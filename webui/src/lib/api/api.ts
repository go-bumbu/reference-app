import { apiClient } from './client'
import type { Info, Me, Note, NoteInput, Tag, TagInput, TagPatch } from './types'

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
