<script setup lang="ts">
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Textarea from 'primevue/textarea'
import { useToast } from 'primevue/usetoast'
import { useCreateNote, useMe, useNotes } from '@/composables/useApi'
import { apiErrorMessage, apiErrorStatus, fieldErrors } from '@/lib/apiError'
import { formatDate } from '@/lib/format'

const toast = useToast()
const { data: me } = useMe()
const { data: notes, isPending, error } = useNotes()
const create = useCreateNote()

const title = ref('')
const body = ref('')

// Field messages come from the server's 422 (the module owns validation);
// anything else is a toast.
const errors = computed(() =>
    apiErrorStatus(create.error.value) === 422 ? fieldErrors(create.error.value) : {}
)

function submit() {
    create.mutate(
        { title: title.value, body: body.value },
        {
            onSuccess: () => {
                title.value = ''
                body.value = ''
            },
            onError: (err) => {
                if (apiErrorStatus(err) !== 422) {
                    toast.add({ severity: 'error', summary: apiErrorMessage(err), life: 5000 })
                }
            }
        }
    )
}

function author(createdBy: string): string {
    return createdBy === me.value?.userId ? 'you' : 'another member'
}
</script>

<template>
    <section class="create">
        <h1>Notes</h1>
        <p class="muted">Notes are shared with everyone in your tenant.</p>
        <form class="form" @submit.prevent="submit">
            <label for="note-title">Title</label>
            <InputText
                id="note-title"
                v-model="title"
                :invalid="!!errors.title"
                maxlength="200"
                data-test="title"
            />
            <small v-if="errors.title" class="error" data-test="title-error">{{
                errors.title
            }}</small>

            <label for="note-body">Body</label>
            <Textarea id="note-body" v-model="body" :invalid="!!errors.body" rows="3" autoResize />
            <small v-if="errors.body" class="error">{{ errors.body }}</small>

            <Button
                type="submit"
                label="Add note"
                :loading="create.isPending.value"
                class="submit"
                data-test="submit"
            />
        </form>
    </section>

    <section>
        <p v-if="isPending" class="muted">Loading…</p>
        <Message v-else-if="error" severity="error">{{ apiErrorMessage(error) }}</Message>
        <p v-else-if="!notes?.length" class="muted" data-test="empty">No notes yet.</p>
        <ul v-else class="list">
            <li v-for="note in notes" :key="note.id" class="item" data-test="note">
                <RouterLink :to="{ name: 'note', params: { id: note.id } }" class="title">
                    {{ note.title }}
                </RouterLink>
                <p v-if="note.body" class="body">{{ note.body }}</p>
                <small class="muted"
                    >by {{ author(note.createdBy) }}, {{ formatDate(note.createdAt) }}</small
                >
            </li>
        </ul>
    </section>
</template>

<style scoped>
h1 {
    margin: 0 0 0.25rem;
}

.form {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    margin: 1rem 0 2rem;
}

.submit {
    align-self: flex-start;
    margin-top: 0.5rem;
}

.error {
    color: var(--c-red-500);
}

.list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
}

.item {
    padding: 0.75rem 1rem;
    background: var(--app-panel);
    border: 1px solid var(--app-border);
    border-radius: 0.5rem;
}

.title {
    font-weight: 600;
}

.body {
    margin: 0.35rem 0;
    white-space: pre-wrap;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
}
</style>
