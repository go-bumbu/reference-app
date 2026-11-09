<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import { useToast } from 'primevue/usetoast'
import { useCreateNote, useUpdateNote } from '@/composables/useApi'
import { apiErrorMessage, apiErrorStatus, fieldErrors } from '@/lib/apiError'
import type { Note } from '@/lib/api/types'

// The note form, in a dialog. Without a note it creates one (the top bar's Add
// note, from any page); given a note it edits that one (the note page's Edit).
// Both mutations invalidate the notes list and refresh the note's own query, so
// the card grid and the open note pick the change up.
const props = defineProps<{ note?: Note }>()
const visible = defineModel<boolean>('visible', { required: true })

const toast = useToast()
const create = useCreateNote()
const update = useUpdateNote()

const editing = computed(() => !!props.note)
const mutation = computed(() => (editing.value ? update : create))

// Unique per instance: the shell's create dialog and a note's edit dialog can
// both be mounted at once.
const formId = useId()

const title = ref('')
const body = ref('')

// Field messages come from the server's 422 (the module owns validation);
// anything else is a toast.
const errors = computed(() => {
    const err = mutation.value.error.value
    return apiErrorStatus(err) === 422 ? fieldErrors(err) : {}
})

// Each opening starts from the note being edited (or blank) and with no error
// from the last attempt.
watch(
    visible,
    (open) => {
        if (!open) return
        title.value = props.note?.title ?? ''
        body.value = props.note?.body ?? ''
        mutation.value.reset()
    },
    { immediate: true }
)

function onError(err: Error) {
    if (apiErrorStatus(err) !== 422) {
        toast.add({ severity: 'error', summary: apiErrorMessage(err), life: 5000 })
    }
}

function done(summary: string) {
    visible.value = false
    toast.add({ severity: 'success', summary, life: 3000 })
}

function submit() {
    const input = { title: title.value, body: body.value }
    if (props.note) {
        update.mutate(
            { id: props.note.id, input },
            { onSuccess: () => done('Note saved'), onError }
        )
    } else {
        create.mutate(input, { onSuccess: () => done('Note added'), onError })
    }
}
</script>

<template>
    <Dialog
        v-model:visible="visible"
        modal
        :header="editing ? 'Edit note' : 'New note'"
        :style="{ width: '32rem' }"
        :breakpoints="{ '640px': '95vw' }"
        :draggable="false"
    >
        <form :id="formId" class="form" data-test="note-form" @submit.prevent="submit">
            <label :for="`${formId}-title`">Title</label>
            <InputText
                :id="`${formId}-title`"
                v-model="title"
                :invalid="!!errors.title"
                maxlength="200"
                autofocus
                data-test="title"
            />
            <small v-if="errors.title" class="error" data-test="title-error">{{
                errors.title
            }}</small>

            <label :for="`${formId}-body`">Body</label>
            <Textarea
                :id="`${formId}-body`"
                v-model="body"
                :invalid="!!errors.body"
                rows="5"
                autoResize
                data-test="body"
            />
            <small v-if="errors.body" class="error">{{ errors.body }}</small>
        </form>
        <template #footer>
            <Button label="Cancel" severity="secondary" text @click="visible = false" />
            <Button
                type="submit"
                :form="formId"
                :label="editing ? 'Save' : 'Add note'"
                :loading="mutation.isPending.value"
                data-test="submit"
            />
        </template>
    </Dialog>
</template>

<style scoped>
.form {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
}

.error {
    color: var(--c-red-500);
}
</style>
