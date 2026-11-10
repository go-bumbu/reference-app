<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import TagRow from '@/components/tags/TagRow.vue'
import {
    useCreateTag,
    useDeleteTag,
    useSetNoteTag,
    useTags,
    useUpdateTag
} from '@/composables/useApi'
import { useTagDrag } from '@/composables/useTagDrag'
import type { Note, Tag, TagPatch } from '@/lib/api/types'
import { apiErrorMessage, apiErrorStatus } from '@/lib/apiError'
import { descendantCount, filterTags, lastChildId, moveTargets, tagPath } from '@/lib/tagTree'

// The tag editor, after codex's tag dialog: the one place the tenant's tag tree
// is made and changed. It lists every tag in tree order; the filter at the top
// narrows them to those whose name holds what is typed, and Enter there makes a
// tag of that name when none at the top level is called so. Each row (TagRow)
// has its actions on its right: + adds a tag below it, the pencil renames it in
// place, the arrows move it under another tag (the keyboard's and the touch
// screen's way to move one), the trash deletes it with everything below it,
// after a confirmation in the row. A row dragged onto another nests under it or
// goes before or after it (useTagDrag); the strip at the foot of the list takes
// a tag back to the top level. Each change is saved at once.
//
// Given a note, the dialog picks that note's tags as well: a click on a tag's
// box adds it to the note or takes it off, and a tag made here is added to the
// note too. Without a note (the sidebar's pencil) it edits the tree alone.
const props = defineProps<{ note?: Note }>()
const visible = defineModel<boolean>('visible', { required: true })

const id = useId()
const { data: tags, isPending, error: loadError } = useTags()
const create = useCreateTag()
const update = useUpdateTag()
const remove = useDeleteTag()
const setTag = useSetNoteTag()

const tree = computed(() => tags.value ?? [])
const header = computed(() => (props.note ? `Tags of “${props.note.title}”` : 'Edit tags'))

// --- the list ------------------------------------------------------------------
const filter = ref('')
const filtering = computed(() => filter.value.trim() !== '')
const rows = computed(() => filterTags(tree.value, filter.value))
// What Enter in the filter makes: its text, unless a tag at the root has it already.
const filterCreates = computed(() => {
    const name = filter.value.trim()
    return name !== '' && !tree.value.some((t) => t.name === name) ? name : ''
})

// --- focus -----------------------------------------------------------------------
// A field or a confirmation that closes gives the focus back to the button of
// the row that opened it, so a keyboard user keeps their place in the dialog.
const rowId = (tagId: number) => `${id}-tag-${tagId}`

function focus(elementId: string) {
    void nextTick(() => document.getElementById(elementId)?.focus())
}

// --- what went wrong ---------------------------------------------------------------
// The last failed change, said above the list until the next change or a close.
const failure = ref('')

function fail(err: unknown) {
    failure.value =
        apiErrorStatus(err) === 409
            ? 'A tag next to it already has that name.'
            : apiErrorMessage(err)
}

// --- picking the note's tags -----------------------------------------------------
const carried = computed(() => new Set(props.note?.tagIds ?? []))

// One pick at a time: the boxes wait while a pick is saved, so a second click
// reads what the first one did instead of the note as it was.
async function setPick(tag: Tag, add: boolean) {
    if (!props.note) return
    failure.value = ''
    try {
        await setTag.mutateAsync({ noteId: props.note.id, tagId: tag.id, add })
    } catch (err) {
        fail(err)
    }
}

// --- adding a tag ------------------------------------------------------------------
// null: no field open below a tag (the field at the top level is always there).
const addingUnder = ref<number | null>(null)
const addDraft = ref('')
const rootDraft = ref('')

function startAdd(tag: Tag) {
    closeFields()
    addingUnder.value = tag.id
    addDraft.value = ''
    focus(`${id}-add`)
}

function cancelAdd() {
    const parent = addingUnder.value
    addingUnder.value = null
    if (parent !== null) focus(`${rowId(parent)}-add`)
}

// A new tag goes last among its siblings, and onto the note when there is one.
async function createTag(name: string, parentId: number): Promise<boolean> {
    failure.value = ''
    try {
        const tag = await create.mutateAsync({
            name,
            parentId,
            afterId: lastChildId(tree.value, parentId)
        })
        if (props.note) await setPick(tag, true)
        return true
    } catch (err) {
        fail(err)
        return false
    }
}

async function submitAdd() {
    const name = addDraft.value.trim()
    const parent = addingUnder.value
    if (!name || parent === null) return
    if (await createTag(name, parent)) cancelAdd()
}

async function submitRoot() {
    const name = rootDraft.value.trim()
    if (name && (await createTag(name, 0))) rootDraft.value = ''
}

async function submitFilter() {
    if (filterCreates.value && (await createTag(filterCreates.value, 0))) filter.value = ''
}

// --- renaming ----------------------------------------------------------------------
const renamingId = ref<number | null>(null)
const renameDraft = ref('')

function startRename(tag: Tag) {
    closeFields()
    renamingId.value = tag.id
    renameDraft.value = tag.name
    focus(`${id}-rename`)
}

function cancelRename() {
    const tagId = renamingId.value
    renamingId.value = null
    if (tagId !== null) focus(`${rowId(tagId)}-rename`)
}

async function submitRename(tag: Tag) {
    const name = renameDraft.value.trim()
    if (!name || name === tag.name) return cancelRename()
    if (await save(tag.id, { name })) cancelRename()
}

// --- moving without a drag -----------------------------------------------------------
// The tag whose row shows where it can move: the top level or another tag.
const movingId = ref<number | null>(null)
const moveTo = ref('')
const targets = computed(() =>
    movingId.value === null ? [] : moveTargets(tree.value, movingId.value)
)

function startMove(tag: Tag) {
    closeFields()
    movingId.value = tag.id
    moveTo.value = ''
    focus(`${id}-move`)
}

function cancelMove() {
    const tagId = movingId.value
    movingId.value = null
    if (tagId !== null) focus(`${rowId(tagId)}-move`)
}

async function submitMove(tag: Tag) {
    if (moveTo.value === '') return
    if (await save(tag.id, { parentId: Number(moveTo.value) })) cancelMove()
}

// --- deleting ----------------------------------------------------------------------
// The tag whose row asks to confirm its delete.
const confirmingId = ref<number | null>(null)

function askDelete(tag: Tag) {
    closeFields()
    confirmingId.value = tag.id
    focus(`${id}-delete-cancel`)
}

function cancelDelete() {
    const tagId = confirmingId.value
    confirmingId.value = null
    if (tagId !== null) focus(`${rowId(tagId)}-delete`)
}

function deleteQuestion(tag: Tag): string {
    const below = descendantCount(tag)
    const also =
        below === 0 ? '' : below === 1 ? ' and the tag below it' : ` and the ${below} tags below it`
    return `Delete “${tag.name}”${also}? Notes keep, without these tags.`
}

async function confirmDelete(tag: Tag) {
    failure.value = ''
    try {
        await remove.mutateAsync(tag.id)
        confirmingId.value = null
        focus(`${id}-filter`)
    } catch (err) {
        fail(err)
    }
}

// --- saving a change of a tag ------------------------------------------------------
async function save(tagId: number, patch: TagPatch): Promise<boolean> {
    failure.value = ''
    try {
        await update.mutateAsync({ id: tagId, patch })
        return true
    } catch (err) {
        fail(err)
        return false
    }
}

// --- moving by drag ------------------------------------------------------------------
const drag = useTagDrag(tree, (tagId, patch) => void save(tagId, patch))
const { draggedId, drop, rootDrop } = drag

// Rows move by drag only while the whole tree shows and no field is open.
const canDrag = computed(() => !filtering.value && !fieldOpen.value)

// --- opening and closing ------------------------------------------------------------
// One field at a time: a rename, a new tag below one, a move, or a delete to confirm.
const fieldOpen = computed(
    () =>
        renamingId.value !== null ||
        addingUnder.value !== null ||
        movingId.value !== null ||
        confirmingId.value !== null
)

function closeFields() {
    renamingId.value = null
    addingUnder.value = null
    movingId.value = null
    confirmingId.value = null
}

// Each opening starts clean: no filter, no field open, no drag, nothing failed.
watch(
    visible,
    (open) => {
        drag.reset()
        if (!open) return
        filter.value = ''
        failure.value = ''
        closeFields()
    },
    { immediate: true }
)

const busy = computed(
    () =>
        create.isPending.value ||
        update.isPending.value ||
        remove.isPending.value ||
        setTag.isPending.value
)
</script>

<template>
    <Dialog
        v-model:visible="visible"
        modal
        :header="header"
        :style="{ width: '34rem' }"
        :breakpoints="{ '640px': '95vw' }"
        :draggable="false"
        data-test="tags-dialog"
    >
        <div class="tags-editor" :aria-busy="busy">
            <InputText
                :id="`${id}-filter`"
                v-model="filter"
                class="filter"
                placeholder="Filter tags, or type a new one…"
                aria-label="Filter tags"
                autofocus
                data-test="tag-filter"
                @keydown.enter.prevent="submitFilter"
            />

            <Message
                v-if="failure"
                severity="error"
                closable
                role="alert"
                data-test="tags-failure"
                @close="failure = ''"
                >{{ failure }}</Message
            >
            <Message v-if="loadError" severity="error">{{ apiErrorMessage(loadError) }}</Message>
            <p v-else-if="isPending" class="muted"><i class="pi pi-spin pi-spinner" /></p>

            <template v-else>
                <p v-if="!tree.length" class="muted" data-test="tags-none">
                    No tags yet. Add the first one below.
                </p>
                <p v-else-if="!rows.length" class="muted" data-test="tags-no-match">
                    No tag matches “{{ filter.trim() }}”.
                </p>

                <ul v-if="rows.length" class="tag-rows" :aria-label="header">
                    <template v-for="{ tag, depth, path } in rows" :key="tag.id">
                        <li
                            class="tag-row"
                            :class="{
                                dragging: draggedId === tag.id,
                                [`drop-${drop?.pos}`]: drop?.id === tag.id
                            }"
                            :style="{ paddingLeft: `${0.5 + (filtering ? 0 : depth) * 1.25}rem` }"
                            :draggable="canDrag"
                            data-test="tag-row"
                            :data-tag-id="tag.id"
                            @dragstart="drag.onDragStart($event, tag)"
                            @dragend="drag.onDragEnd"
                            @dragover="drag.onDragOver($event, tag)"
                            @dragleave="drag.onDragLeave($event, tag)"
                            @drop="drag.onDrop($event, tag)"
                        >
                            <!-- confirming a delete -->
                            <template v-if="confirmingId === tag.id">
                                <span class="confirm-text" role="alert">{{
                                    deleteQuestion(tag)
                                }}</span>
                                <span class="row-buttons">
                                    <Button
                                        label="Delete"
                                        severity="danger"
                                        size="small"
                                        :loading="remove.isPending.value"
                                        data-test="confirm-delete"
                                        @click="confirmDelete(tag)"
                                    />
                                    <Button
                                        :id="`${id}-delete-cancel`"
                                        label="Cancel"
                                        severity="secondary"
                                        size="small"
                                        text
                                        @click="cancelDelete"
                                    />
                                </span>
                            </template>

                            <!-- renaming -->
                            <form
                                v-else-if="renamingId === tag.id"
                                class="inline-form"
                                @submit.prevent="submitRename(tag)"
                            >
                                <InputText
                                    :id="`${id}-rename`"
                                    v-model="renameDraft"
                                    size="small"
                                    maxlength="100"
                                    :aria-label="`New name of ${path}`"
                                    data-test="rename-input"
                                    @keydown.esc.stop.prevent="cancelRename"
                                />
                                <Button
                                    type="submit"
                                    icon="pi pi-check"
                                    size="small"
                                    text
                                    aria-label="Save the name"
                                    :loading="update.isPending.value"
                                    data-test="rename-save"
                                />
                            </form>

                            <!-- moving: where to -->
                            <form
                                v-else-if="movingId === tag.id"
                                class="inline-form"
                                @submit.prevent="submitMove(tag)"
                            >
                                <select
                                    :id="`${id}-move`"
                                    v-model="moveTo"
                                    class="move-select"
                                    :aria-label="`Move ${path} under`"
                                    data-test="move-select"
                                    @keydown.esc.stop.prevent="cancelMove"
                                >
                                    <option value="" disabled>Move under…</option>
                                    <option
                                        v-for="target in targets"
                                        :key="target.id"
                                        :value="String(target.id)"
                                    >
                                        {{ target.label }}
                                    </option>
                                </select>
                                <Button
                                    type="submit"
                                    icon="pi pi-check"
                                    size="small"
                                    text
                                    aria-label="Move the tag"
                                    :disabled="moveTo === ''"
                                    :loading="update.isPending.value"
                                    data-test="move-save"
                                />
                                <Button
                                    icon="pi pi-times"
                                    size="small"
                                    text
                                    severity="secondary"
                                    aria-label="Cancel the move"
                                    @click="cancelMove"
                                />
                            </form>

                            <!-- the tag -->
                            <TagRow
                                v-else
                                :tag="tag"
                                :label="filtering ? path : tag.name"
                                :path="path"
                                :id-prefix="rowId(tag.id)"
                                :picking="!!note"
                                :carried="carried.has(tag.id)"
                                :pick-disabled="setTag.isPending.value"
                                :movable="canDrag"
                                @pick="setPick(tag, !carried.has(tag.id))"
                                @add="startAdd(tag)"
                                @rename="startRename(tag)"
                                @move="startMove(tag)"
                                @delete="askDelete(tag)"
                            />
                        </li>

                        <!-- a new tag below this one -->
                        <li
                            v-if="addingUnder === tag.id"
                            class="tag-row adding"
                            :style="{
                                paddingLeft: `${0.5 + ((filtering ? 0 : depth) + 1) * 1.25}rem`
                            }"
                        >
                            <form class="inline-form" @submit.prevent="submitAdd">
                                <InputText
                                    :id="`${id}-add`"
                                    v-model="addDraft"
                                    size="small"
                                    maxlength="100"
                                    :placeholder="`New tag in ${tagPath(tree, tag.id)}`"
                                    :aria-label="`New tag under ${path}`"
                                    data-test="add-child-input"
                                    @keydown.esc.stop.prevent="cancelAdd"
                                />
                                <Button
                                    type="submit"
                                    icon="pi pi-check"
                                    size="small"
                                    text
                                    aria-label="Add the tag"
                                    :loading="create.isPending.value"
                                    data-test="add-child-save"
                                />
                            </form>
                        </li>
                    </template>
                </ul>

                <!-- while a tag is dragged: back to the top level -->
                <div
                    v-if="draggedId !== null"
                    class="root-drop"
                    :class="{ over: rootDrop }"
                    data-test="root-drop"
                    @dragover="drag.onRootDragOver"
                    @dragleave="drag.onRootDragLeave"
                    @drop="drag.onRootDrop"
                >
                    Drop here to move it to the top level
                </div>

                <button
                    v-if="filterCreates"
                    type="button"
                    class="create-from-filter"
                    data-test="create-from-filter"
                    @click="submitFilter"
                >
                    <i class="pi pi-plus" aria-hidden="true" /> Create “{{ filterCreates }}”
                </button>

                <form class="root-form" @submit.prevent="submitRoot">
                    <InputText
                        v-model="rootDraft"
                        size="small"
                        maxlength="100"
                        placeholder="New tag at the top level"
                        aria-label="New tag at the top level"
                        data-test="new-root-tag"
                    />
                    <Button
                        type="submit"
                        label="Add"
                        icon="pi pi-plus"
                        size="small"
                        :disabled="!rootDraft.trim()"
                        :loading="create.isPending.value && addingUnder === null"
                        data-test="new-root-submit"
                    />
                </form>
            </template>
        </div>
        <template #footer>
            <Button label="Done" data-test="tags-done" @click="visible = false" />
        </template>
    </Dialog>
</template>

<style scoped>
.tags-editor {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
}

.filter {
    width: 100%;
}

.tag-rows {
    display: flex;
    flex-direction: column;
    max-height: 50vh;
    margin: 0;
    padding: 0.25rem 0;
    overflow-y: auto;
    list-style: none;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius);
}

.tag-row {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    min-height: 2.25rem;
    padding-right: 0.35rem;
    border-radius: var(--app-radius);
    font-size: 0.9rem;
}
.tag-row:hover {
    background: var(--app-hover);
}
/* A row's actions (TagRow) show while the row is hovered or holds the focus. */
.tag-row:hover :deep(.row-actions),
.tag-row:focus-within :deep(.row-actions) {
    opacity: 1;
}
.tag-row.dragging {
    opacity: 0.45;
}
/* Where a dragged tag lands: a line above or below the row, or the row itself
   for nesting under it. */
.tag-row.drop-before {
    box-shadow: inset 0 2px 0 var(--app-accent);
}
.tag-row.drop-after {
    box-shadow: inset 0 -2px 0 var(--app-accent);
}
.tag-row.drop-onto {
    background: var(--app-accent-soft);
    box-shadow: inset 0 0 0 2px var(--app-accent);
}

.confirm-text {
    flex: 1;
    min-width: 0;
    font-size: 0.85rem;
}

.row-buttons {
    display: inline-flex;
    align-items: center;
    gap: 0.15rem;
    margin-left: auto;
}

.inline-form,
.root-form {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    min-width: 0;
}
.inline-form :deep(input),
.root-form :deep(input),
.move-select {
    flex: 1;
    min-width: 0;
}

.move-select {
    height: 2rem;
    padding: 0 0.5rem;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius);
    background: var(--app-panel);
    color: var(--app-text-primary);
    font: inherit;
    font-size: 0.85rem;
}
.move-select:focus-visible {
    outline: none;
    box-shadow: 0 0 0 2px var(--app-focus-ring);
}

.root-drop {
    padding: 0.6rem;
    border: 1px dashed var(--app-border);
    border-radius: var(--app-radius);
    color: var(--app-text-muted);
    font-size: 0.85rem;
    text-align: center;
}
.root-drop.over {
    border-color: var(--app-accent);
    background: var(--app-accent-soft);
    color: var(--app-text-primary);
}

.create-from-filter {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.4rem 0.5rem;
    border: none;
    border-radius: var(--app-radius);
    background: var(--app-accent-soft);
    color: var(--app-text-primary);
    font: inherit;
    font-size: 0.9rem;
    text-align: left;
    cursor: pointer;
}
.create-from-filter:focus-visible {
    outline: none;
    box-shadow: 0 0 0 2px var(--app-focus-ring);
}
</style>
