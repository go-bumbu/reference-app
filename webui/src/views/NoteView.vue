<script setup lang="ts">
import Message from 'primevue/message'
import { useNote } from '@/composables/useApi'
import { apiErrorMessage, apiErrorStatus } from '@/lib/apiError'
import { formatDate } from '@/lib/format'

const props = defineProps<{ id: string }>()
const { data: note, isPending, error } = useNote(() => props.id)
</script>

<template>
    <RouterLink to="/">← All notes</RouterLink>
    <p v-if="isPending" class="muted">Loading…</p>
    <!-- another tenant's note is a 404 too: the API does not tell them apart -->
    <Message v-else-if="apiErrorStatus(error) === 404" severity="warn" data-test="not-found">
        This note does not exist.
    </Message>
    <Message v-else-if="error" severity="error">{{ apiErrorMessage(error) }}</Message>
    <article v-else-if="note">
        <h1>{{ note.title }}</h1>
        <small class="muted">{{ formatDate(note.createdAt) }}</small>
        <p class="body">{{ note.body }}</p>
    </article>
</template>

<style scoped>
article h1 {
    margin: 1rem 0 0.25rem;
}

.body {
    white-space: pre-wrap;
}
</style>
