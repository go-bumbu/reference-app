<script setup lang="ts">
import { computed } from 'vue'
import Message from 'primevue/message'
import { useRoute } from 'vue-router'
import { useMe, useNotes } from '@/composables/useApi'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDate } from '@/lib/format'

// The notes as a card grid. Creating one is the top bar's Add note
// (NoteDialog), so this page only shows them.
const route = useRoute()
const { data: me } = useMe()
const { data: notes, isPending, error } = useNotes()

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
    <p v-if="isPending" class="muted">Loading…</p>
    <Message v-else-if="error" severity="error">{{ apiErrorMessage(error) }}</Message>
    <p v-else-if="!notes?.length" class="muted" data-test="empty">
        No notes yet. Use <strong>Add note</strong> in the top bar to write the first one.
    </p>
    <p v-else-if="!shown.length" class="muted" data-test="no-match">
        No notes match “{{ route.query.q }}”.
    </p>
    <ul v-else class="grid">
        <li v-for="note in shown" :key="note.id" data-test="note">
            <RouterLink :to="{ name: 'note', params: { id: note.id } }" class="card">
                <h2 class="card-title">{{ note.title }}</h2>
                <p v-if="note.body" class="card-body">{{ note.body }}</p>
                <small class="card-meta muted"
                    >by {{ author(note.createdBy) }}, {{ formatDate(note.createdAt) }}</small
                >
            </RouterLink>
        </li>
    </ul>
</template>

<style scoped>
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
