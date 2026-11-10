<script setup lang="ts">
import { computed } from 'vue'
import Message from 'primevue/message'
import { useRoute } from 'vue-router'
import { useMe, useNotes, useTags } from '@/composables/useApi'
import { useNoteDrag } from '@/composables/useNoteDrag'
import type { Note } from '@/lib/api/types'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDate } from '@/lib/format'
import { flattenTags, tagIdFromQuery } from '@/lib/tagTree'

// The notes as a card grid. Creating one is the top bar's Add note
// (NoteDialog), so this page only shows them. A tag of the sidebar narrows the
// grid to the notes under it (`?tag=`, the server does the narrowing); a card
// dragged onto a tag of the sidebar gets that tag.
const route = useRoute()
const { data: me } = useMe()
const tagId = computed(() => tagIdFromQuery(route.query.tag))
const { data: notes, isPending, error } = useNotes(tagId)
const { data: tags } = useTags()
const drag = useNoteDrag()

// Every tag by id, with its path, for the cards' chips and the heading.
const tagsById = computed(() => new Map(flattenTags(tags.value ?? []).map((f) => [f.tag.id, f])))
const activeTag = computed(() =>
    tagId.value === null ? undefined : tagsById.value.get(tagId.value)
)

function tagsOf(note: Note) {
    return note.tagIds.flatMap((id) => {
        const f = tagsById.value.get(id)
        return f ? [{ id, name: f.tag.name, path: f.path }] : []
    })
}

const allNotes = computed(() => ({
    name: 'notes',
    query: typeof route.query.q === 'string' ? { q: route.query.q } : {}
}))

// The top bar's search (`?q=`) filters the list on the client: the API returns
// every note of the tenant in one page, so there is nothing to ask the server.
const query = computed(() =>
    typeof route.query.q === 'string' ? route.query.q.trim().toLowerCase() : ''
)
const shown = computed(() => {
    const q = query.value
    if (!q) return notes.value ?? []
    return (notes.value ?? []).filter(
        (n) => n.title.toLowerCase().includes(q) || n.body.toLowerCase().includes(q)
    )
})

function author(createdBy: string): string {
    return createdBy === me.value?.userId ? 'you' : 'another member'
}
</script>

<template>
    <div v-if="tagId !== null" class="filter-bar" data-test="tag-filter-bar">
        <i class="pi pi-tag" aria-hidden="true" />
        <span v-if="activeTag"
            >Notes tagged <strong>{{ activeTag.path }}</strong> or a tag below it</span
        >
        <span v-else-if="tags">This tag does not exist any more.</span>
        <RouterLink :to="allNotes" class="filter-clear" data-test="tag-filter-clear"
            >Show all notes</RouterLink
        >
    </div>
    <p v-if="isPending" class="muted">Loading…</p>
    <Message v-else-if="error" severity="error">{{ apiErrorMessage(error) }}</Message>
    <p v-else-if="!notes?.length && tagId !== null" class="muted" data-test="empty-tag">
        No notes under this tag yet. Clear the filter, then drag a note onto the tag in the sidebar
        to add one.
    </p>
    <p v-else-if="!notes?.length" class="muted" data-test="empty">
        No notes yet. Use <strong>Add note</strong> in the top bar to write the first one.
    </p>
    <p v-else-if="!shown.length" class="muted" data-test="no-match">
        No notes match “{{ route.query.q }}”.
    </p>
    <ul v-else class="grid">
        <li v-for="note in shown" :key="note.id" data-test="note">
            <RouterLink
                :to="{ name: 'note', params: { id: note.id } }"
                class="card"
                draggable="true"
                title="Open the note, or drag it onto a tag"
                @dragstart="drag.start($event, note)"
                @dragend="drag.end()"
            >
                <h2 class="card-title">{{ note.title }}</h2>
                <p v-if="note.body" class="card-body">{{ note.body }}</p>
                <ul v-if="tagsOf(note).length" class="card-tags" aria-label="Tags">
                    <li
                        v-for="tag in tagsOf(note)"
                        :key="tag.id"
                        class="chip"
                        :title="tag.path"
                        data-test="note-tag"
                    >
                        {{ tag.name }}
                    </li>
                </ul>
                <small class="card-meta muted"
                    >by {{ author(note.createdBy) }}, {{ formatDate(note.createdAt) }}</small
                >
            </RouterLink>
        </li>
    </ul>
</template>

<style scoped>
.filter-bar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-bottom: 1rem;
    color: var(--app-text-muted);
    font-size: 0.9rem;
}
.filter-bar strong {
    color: var(--app-text-primary);
}
.filter-clear {
    margin-left: auto;
    color: var(--app-accent);
    font-size: 0.85rem;
}

.card-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
    margin: 0;
    padding: 0;
    list-style: none;
}

.chip {
    max-width: 100%;
    padding: 0.1rem 0.55rem;
    border-radius: 999px;
    background: var(--app-accent-soft);
    color: var(--app-text-primary);
    font-size: 0.75rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
    gap: 1rem;
    margin: 0;
    padding: 0;
    list-style: none;
}

/* The whole card is the link to the note's page. */
.card {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    height: 100%;
    min-height: 9rem;
    padding: 1rem 1.1rem;
    background: var(--app-panel);
    border: 1px solid var(--app-border);
    border-radius: 0.5rem;
    color: var(--app-text-primary);
    text-decoration: none;
    transition:
        border-color 0.15s,
        box-shadow 0.15s,
        transform 0.15s;
}
.card:hover {
    border-color: var(--app-accent);
    box-shadow: 0 4px 14px rgb(0 0 0 / 0.08);
    transform: translateY(-1px);
}
.card:focus-visible {
    outline: 2px solid var(--app-focus-ring);
    outline-offset: 2px;
}

.card-title {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    overflow-wrap: anywhere;
}

.card-body {
    flex: 1;
    margin: 0;
    font-size: 0.9rem;
    white-space: pre-wrap;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 5;
    -webkit-box-orient: vertical;
}

/* Pinned to the card's foot, whatever the body's length. */
.card-meta {
    margin-top: auto;
    font-size: 0.75rem;
}
</style>
