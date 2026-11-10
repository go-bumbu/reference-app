<script setup lang="ts">
import type { Tag } from '@/lib/api/types'

// One tag of the tag editor (TagsDialog), at rest: its name (or, given a note,
// a box that says whether the note has the tag) and its actions, which show on
// hover or focus. The dialog owns what the actions open: the fields to add,
// rename and move, and the delete's confirmation. Each action button has an id
// (`${idPrefix}-<action>`), so the dialog can give it the focus back when what it
// opened closes.
defineProps<{
    tag: Tag
    // the name, or the path while the list is filtered
    label: string
    // the path: the accessible name, as siblings of other tags share names
    path: string
    idPrefix: string
    // given a note, the row picks the tag for it
    picking: boolean
    carried: boolean
    pickDisabled: boolean
    // whether the row can be dragged (the grip shows)
    movable: boolean
}>()
defineEmits<{ pick: []; add: []; rename: []; move: []; delete: [] }>()
</script>

<template>
    <i v-if="movable" class="pi pi-bars grip" aria-hidden="true" title="Drag to move" />
    <button
        v-if="picking"
        type="button"
        class="pick"
        role="checkbox"
        :aria-checked="carried"
        :aria-label="path"
        :disabled="pickDisabled"
        :title="carried ? 'Take the tag off the note' : 'Add the tag to the note'"
        data-test="tag-pick"
        @click="$emit('pick')"
    >
        <i :class="carried ? 'pi pi-check-square' : 'pi pi-stop'" aria-hidden="true" />
        <span>{{ label }}</span>
    </button>
    <span v-else class="tag-name" :title="path">
        <i class="pi pi-tag" aria-hidden="true" />
        <span>{{ label }}</span>
    </span>
    <span class="row-actions">
        <button
            :id="`${idPrefix}-add`"
            type="button"
            class="row-btn"
            :aria-label="`Add a tag under ${path}`"
            title="Add a tag below"
            data-test="tag-add-child"
            @click="$emit('add')"
        >
            <i class="pi pi-plus" aria-hidden="true" />
        </button>
        <button
            :id="`${idPrefix}-rename`"
            type="button"
            class="row-btn"
            :aria-label="`Rename ${path}`"
            title="Rename"
            data-test="tag-rename"
            @click="$emit('rename')"
        >
            <i class="pi pi-pencil" aria-hidden="true" />
        </button>
        <button
            :id="`${idPrefix}-move`"
            type="button"
            class="row-btn"
            :aria-label="`Move ${path}`"
            title="Move to another tag"
            data-test="tag-move"
            @click="$emit('move')"
        >
            <i class="pi pi-arrow-right-arrow-left" aria-hidden="true" />
        </button>
        <button
            :id="`${idPrefix}-delete`"
            type="button"
            class="row-btn danger"
            :aria-label="`Delete ${path}`"
            title="Delete"
            data-test="tag-delete"
            @click="$emit('delete')"
        >
            <i class="pi pi-trash" aria-hidden="true" />
        </button>
    </span>
</template>

<style scoped>
.grip {
    flex-shrink: 0;
    font-size: 0.7rem;
    color: var(--app-text-muted);
    cursor: grab;
}

.tag-name,
.pick {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
}
.tag-name span,
.pick span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.tag-name i {
    font-size: 0.8rem;
    color: var(--app-text-muted);
}
.pick {
    padding: 0.25rem 0;
    border: none;
    background: none;
    color: var(--app-text-primary);
    font: inherit;
    text-align: left;
    cursor: pointer;
}
.pick:disabled {
    cursor: progress;
}
.pick i {
    color: var(--app-accent);
}
.pick:focus-visible {
    outline: none;
    box-shadow: 0 0 0 2px var(--app-focus-ring);
    border-radius: var(--app-radius);
}

/* The actions show on hover or focus of the row (TagsDialog shows them), and
   always on touch screens. */
.row-actions {
    display: inline-flex;
    align-items: center;
    gap: 0.15rem;
    margin-left: auto;
    opacity: 0;
    transition: opacity 0.15s;
}
@media (hover: none) {
    .row-actions {
        opacity: 1;
    }
}

.row-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.75rem;
    height: 1.75rem;
    border: none;
    border-radius: var(--app-radius);
    background: transparent;
    color: var(--app-text-muted);
    font-size: 0.8rem;
    cursor: pointer;
}
.row-btn:hover {
    color: var(--app-accent);
}
.row-btn.danger:hover {
    color: var(--app-danger);
}
.row-btn:focus-visible {
    outline: none;
    box-shadow: 0 0 0 2px var(--app-focus-ring);
}
</style>
