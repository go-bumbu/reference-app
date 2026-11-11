<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import type { JobRun } from '@/lib/api/types'
import { useJobRunLog } from '@/composables/useApi'
import { apiErrorMessage } from '@/lib/apiError'
import { formatDate } from '@/lib/format'
import { runDuration, statusLabel, statusSeverity } from '@/lib/jobs'

// A run's log, after aether's LogViewer: polled while the dialog is open and
// kept scrolled to its end, so a running job's lines show as they come.
const props = defineProps<{
    run: JobRun | null
    title?: string
}>()

const visible = defineModel<boolean>('visible', { required: true })

const runId = computed(() => props.run?.id ?? '')
const { data: logText, isError, error } = useJobRunLog(runId, visible)

const logBox = ref<HTMLElement | null>(null)
watch(logText, async () => {
    await nextTick()
    if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight
})
</script>

<template>
    <Dialog
        v-model:visible="visible"
        :header="title ? `Run log: ${title}` : 'Run log'"
        modal
        :style="{ width: '90vw', maxWidth: '64rem' }"
        :breakpoints="{ '640px': '100vw' }"
    >
        <div v-if="run" class="run-meta" data-test="run-log-meta">
            <Tag :value="statusLabel(run.status)" :severity="statusSeverity(run.status)" />
            <span>Queued {{ formatDate(run.queuedAt) }}</span>
            <span v-if="run.startedAt">· Started {{ formatDate(run.startedAt) }}</span>
            <span v-if="run.endedAt">· Took {{ runDuration(run) }}</span>
        </div>
        <Message v-if="isError" severity="error" :closable="false">
            {{ apiErrorMessage(error) }}
        </Message>
        <div ref="logBox" class="log-content">
            <pre v-if="logText" data-test="run-log">{{ logText }}</pre>
            <div v-else class="log-empty">
                <i class="pi pi-file" aria-hidden="true"></i>
                <p>No log output</p>
            </div>
        </div>
    </Dialog>
</template>

<style scoped>
.run-meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
    font-size: 0.85rem;
    color: var(--app-text-muted);
}
.log-content {
    max-height: 70vh;
    overflow-y: auto;
    padding: 1rem;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius-lg);
    background: var(--app-background);
}
.log-content pre {
    margin: 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.8rem;
    line-height: 1.6;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
}
.log-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.5rem;
    padding: 2rem;
    color: var(--app-text-muted);
}
.log-empty .pi {
    font-size: 1.5rem;
}
.log-empty p {
    margin: 0;
}
</style>
