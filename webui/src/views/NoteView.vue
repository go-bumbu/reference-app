<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import NoteDialog from '@/components/NoteDialog.vue'
import TagsDialog from '@/components/tags/TagsDialog.vue'
import { useDeleteNote, useMe, useNote, useTags } from '@/composables/useApi'
import { apiErrorMessage, apiErrorStatus } from '@/lib/apiError'
import { formatDate } from '@/lib/format'
import { flattenTags } from '@/lib/tagTree'

// One note, after codex's file and wiki pages: a sheet of paper with the note
// on it and, at its right, a column with the note's details and, at their
// foot, its actions. Edit opens the same dialog as Add note, filled in.
const props = defineProps<{ id: string }>()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const { data: me } = useMe()
const { data: note, isPending, error } = useNote(() => props.id)
const remove = useDeleteNote()

const editOpen = ref(false)
const tagsOpen = ref(false)

// The note's tags, by path, in tree order; a link each to the notes under it.
const { data: tags } = useTags()
const noteTags = computed(() => {
    const carried = new Set(note.value?.tagIds ?? [])
    return flattenTags(tags.value ?? []).filter((f) => carried.has(f.tag.id))
})

const author = computed(() => {
    if (!note.value) return ''
    return note.value.createdBy === me.value?.userId ? 'You' : 'Another member'
})
const words = computed(() => {
    const text = note.value?.body.trim() ?? ''
    return text ? text.split(/\s+/).length : 0
})

function onDelete() {
    confirm.require({
        header: 'Delete note',
        message: `Delete “${note.value?.title}”? Everyone in your tenant loses it; this cannot be undone.`,
        icon: 'pi pi-trash',
        rejectProps: { label: 'Cancel', severity: 'secondary', text: true },
        acceptProps: { label: 'Delete', severity: 'danger' },
        accept: () =>
            remove.mutate(props.id, {
                onSuccess: () => {
                    toast.add({ severity: 'success', summary: 'Note deleted', life: 3000 })
                    router.push({ name: 'notes' })
                },
                onError: (err) => {
                    toast.add({ severity: 'error', summary: apiErrorMessage(err), life: 5000 })
                }
            })
    })
}
</script>

<template>
    <div class="note-page">
        <p v-if="isPending" class="note-state"><i class="pi pi-spin pi-spinner" /></p>
        <!-- another tenant's note is a 404 too: the API does not tell them apart -->
        <Message v-else-if="apiErrorStatus(error) === 404" severity="warn" data-test="not-found">
            This note does not exist.
        </Message>
        <Message v-else-if="error" severity="error">{{ apiErrorMessage(error) }}</Message>
        <div v-else-if="note" class="note-body">
            <article class="paper">
                <h1 class="paper-title">{{ note.title }}</h1>
                <p v-if="note.body" class="paper-text">{{ note.body }}</p>
                <p v-else class="paper-text paper-empty">This note has no text.</p>
            </article>

            <aside class="note-side">
                <section
                    class="side-card detail-panel"
                    aria-label="Note details"
                    data-test="details"
                >
                    <h2 class="detail-name">{{ note.title }}</h2>
                    <span class="detail-type">Note</span>
                    <dl class="detail-rows">
                        <div class="detail-row">
                            <dt class="detail-label">Author</dt>
                            <dd class="detail-value" data-test="detail-author">{{ author }}</dd>
                        </div>
                        <div class="detail-row">
                            <dt class="detail-label">Created</dt>
                            <dd class="detail-value" data-test="detail-created">
                                {{ formatDate(note.createdAt) }}
                            </dd>
                        </div>
                        <div class="detail-row">
                            <dt class="detail-label">Words</dt>
                            <dd class="detail-value" data-test="detail-words">{{ words }}</dd>
                        </div>
                        <div class="detail-row">
                            <dt class="detail-label">Characters</dt>
                            <dd class="detail-value">{{ note.body.length }}</dd>
                        </div>
                        <div class="detail-row">
                            <dt class="detail-label">ID</dt>
                            <dd class="detail-value mono">{{ note.id }}</dd>
                        </div>
                    </dl>
                    <div class="note-actions" aria-label="Note actions" data-test="actions">
                        <Button
                            label="Edit"
                            icon="pi pi-pencil"
                            size="small"
                            class="action-main"
                            data-test="edit"
                            @click="editOpen = true"
                        />
                        <button
                            type="button"
                            class="bar-btn icon-only danger"
                            aria-label="Delete"
                            title="Delete the note"
                            data-test="delete"
                            :disabled="remove.isPending.value"
                            @click="onDelete"
                        >
                            <i
                                :class="
                                    remove.isPending.value ? 'pi pi-spin pi-spinner' : 'pi pi-trash'
                                "
                                aria-hidden="true"
                            ></i>
                        </button>
                    </div>
                </section>
                <section class="side-card tags-panel" aria-label="Note tags" data-test="note-tags">
                    <div class="tags-head">
                        <h2 class="detail-label">Tags</h2>
                        <button
                            type="button"
                            class="bar-btn"
                            title="Add or remove tags"
                            data-test="edit-note-tags"
                            @click="tagsOpen = true"
                        >
                            <i class="pi pi-tags" aria-hidden="true" />&nbsp;Edit
                        </button>
                    </div>
                    <ul v-if="noteTags.length" class="tag-chips">
                        <li v-for="f in noteTags" :key="f.tag.id">
                            <RouterLink
                                :to="{ name: 'notes', query: { tag: String(f.tag.id) } }"
                                class="chip"
                                :title="`Notes tagged ${f.path}`"
                                data-test="note-tag"
                                >{{ f.path }}</RouterLink
                            >
                        </li>
                    </ul>
                    <p v-else class="muted no-tags">No tags.</p>
                </section>
            </aside>
            <NoteDialog v-model:visible="editOpen" :note="note" />
            <TagsDialog v-model:visible="tagsOpen" :note="note" />
        </div>
    </div>
</template>

<style scoped>
.note-page {
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

/* A card of the column at the sheet's right. */
.side-card {
    padding: 1rem;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius-lg);
    background: var(--app-panel);
}

/* --- The note's tags, a card below the details --- */
.tags-panel {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
}

.tags-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
}
.tags-head h2 {
    margin: 0;
    font-size: 0.85rem;
    font-weight: 500;
}

.tag-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin: 0;
    padding: 0;
    list-style: none;
}

.chip {
    display: inline-block;
    max-width: 100%;
    padding: 0.12rem 0.55rem;
    border-radius: 999px;
    background: var(--app-accent-soft);
    color: var(--app-text-primary);
    font-size: 0.8rem;
    font-weight: 500;
    text-decoration: none;
    overflow-wrap: anywhere;
}
.chip:hover {
    color: var(--app-accent);
}
.chip:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px var(--app-focus-ring);
}

.no-tags {
    margin: 0;
    font-size: 0.85rem;
}

/* --- The note's actions, at the foot of the details card: Edit fills the
   row, Delete sits beside it. --- */
.note-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding-top: 0.75rem;
    border-top: 1px solid var(--app-border);
}

.action-main {
    flex: 1;
}

.bar-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    height: 2rem;
    min-width: 2rem;
    padding: 0 0.5rem;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius);
    background: var(--app-panel);
    color: var(--app-text-muted);
    font-size: 0.85rem;
    text-decoration: none;
    cursor: pointer;
    transition:
        border-color 0.15s,
        color 0.15s;
}
.bar-btn.icon-only {
    padding: 0;
    width: 2rem;
}
.bar-btn:hover:not(:disabled) {
    border-color: var(--app-accent);
    color: var(--app-accent);
}
.bar-btn.danger:hover:not(:disabled) {
    border-color: var(--app-danger);
    color: var(--app-danger);
}
.bar-btn:disabled {
    opacity: 0.6;
    cursor: default;
}
.bar-btn:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px var(--app-focus-ring);
}

/* --- The sheet and the details column beside it --- */
.note-body {
    display: flex;
    align-items: flex-start;
    gap: 1.25rem;
}

/* A blank sheet of A4 paper: 210 mm wide where there is room, at least as tall
   as the paper is for its width, growing with a longer note. Its margins are a
   document's, narrowed with the room there is. */
.paper {
    flex: 1;
    min-width: 0;
    max-width: 210mm;
    aspect-ratio: 210 / 297;
    margin: 0 auto 3rem;
    padding: clamp(1.5rem, 7%, 4.5rem) clamp(1.25rem, 8%, 5rem);
    display: flex;
    flex-direction: column;
    background: var(--app-panel);
    border: 1px solid var(--app-border);
    border-radius: 3px;
    box-shadow: var(--app-shadow);
}

.paper-title {
    margin: 0 0 1.5rem;
    padding-bottom: 1.25rem;
    border-bottom: 1px solid var(--app-border);
    font-size: 2rem;
    line-height: 1.2;
    font-weight: 600;
    letter-spacing: -0.02em;
    color: var(--app-text-primary);
    overflow-wrap: anywhere;
}

.paper-text {
    margin: 0;
    font-size: 1rem;
    line-height: 1.65;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    color: var(--app-text-primary);
}

.paper-empty {
    color: var(--app-text-muted);
    font-style: italic;
}

/* The details column, after codex's file details panel. */
.note-side {
    position: sticky;
    top: 0;
    flex: 0 0 17rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
}

.detail-panel {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.5rem;
}

.detail-name {
    margin: 0;
    font-size: 1.05rem;
    font-weight: 600;
    line-height: 1.3;
    overflow-wrap: anywhere;
    color: var(--app-text-primary);
}

.detail-type {
    padding: 0.1rem 0.55rem;
    border-radius: 999px;
    background: var(--app-accent-soft);
    color: var(--app-accent);
    font-size: 0.72rem;
}

.detail-rows {
    width: 100%;
    margin: 0.4rem 0 0;
}

.detail-row {
    display: grid;
    grid-template-columns: 5.5rem minmax(0, 1fr);
    gap: 0.5rem;
    padding: 0.45rem 0;
    border-top: 1px solid var(--app-border);
    font-size: 0.85rem;
}

.detail-label {
    color: var(--app-text-muted);
}

.detail-value {
    margin: 0;
    color: var(--app-text-primary);
    font-variant-numeric: tabular-nums;
    overflow-wrap: anywhere;
}

.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.8rem;
}

.note-state {
    padding: 4rem 1rem;
    text-align: center;
    color: var(--app-text-muted);
}

/* No room for both side by side: the details go under the sheet, and the
   sheet is only as tall as what is on it. */
@media (max-width: 960px) {
    .note-body {
        flex-direction: column;
        align-items: stretch;
    }
    .note-side {
        position: static;
        flex: none;
    }
    .paper {
        width: 100%;
        aspect-ratio: auto;
        min-height: 60vh;
        margin-bottom: 0;
    }
}

@media (max-width: 640px) {
    .paper {
        padding: 1.25rem 1rem 2rem;
    }
    .paper-title {
        font-size: 1.45rem;
    }
}
</style>
