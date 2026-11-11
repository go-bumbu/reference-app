<script setup lang="ts">
import { computed } from 'vue'
import DataTable, { type DataTableRowClickEvent } from 'primevue/datatable'
import Column from 'primevue/column'
import Tag from 'primevue/tag'
import Button from 'primevue/button'
import type { Job, JobRun } from '@/lib/api/types'
import { isActiveStatus, runDuration, statusLabel, statusSeverity } from '@/lib/jobs'
import { formatDate } from '@/lib/format'

// The runs of the tenant, after aether's ExecutionHistory: a count per status
// over a table, newest first. A row (or its info button, the keyboard's way)
// opens the run's log; a waiting or running run can be canceled.
const props = defineProps<{
    runs: JobRun[]
    jobs: Job[]
    loading: boolean
    // the id of the run whose cancel is in flight
    cancelingId?: string | null
}>()

const emit = defineEmits<{
    cancel: [runId: string]
    open: [run: JobRun]
}>()

const counts = computed(() => {
    const by = (s: string) => props.runs.filter((r) => r.status === s).length
    return {
        total: props.runs.length,
        waiting: by('waiting'),
        running: by('running'),
        complete: by('complete'),
        failed: by('failed') + by('panicked')
    }
})

// The job's title for a run, its name if the server no longer defines it.
const jobTitle = (name: string) => props.jobs.find((j) => j.name === name)?.title ?? name
</script>

<template>
    <div>
        <p v-if="!loading" class="runs-summary" data-test="runs-summary">
            <strong>Total: {{ counts.total }}</strong>
            <span v-if="counts.total > 0">
                · Queued: {{ counts.waiting }} · Running: {{ counts.running }} · Complete:
                {{ counts.complete }} · Failed: {{ counts.failed }}
            </span>
        </p>
        <div class="table-fit">
            <DataTable
                :value="runs"
                :loading="loading"
                dataKey="id"
                stripedRows
                size="small"
                class="runs-table"
                @rowClick="(e: DataTableRowClickEvent) => emit('open', e.data as JobRun)"
            >
                <template #empty><div class="empty-table">No runs yet</div></template>
                <Column header="Job">
                    <template #body="{ data }">
                        <span :data-test="`run-${data.id}`">{{ jobTitle(data.job) }}</span>
                    </template>
                </Column>
                <Column header="Queued" headerClass="col-wide" bodyClass="col-wide">
                    <template #body="{ data }">{{ formatDate(data.queuedAt) }}</template>
                </Column>
                <Column header="Duration" headerClass="col-wide" bodyClass="col-wide">
                    <template #body="{ data }">{{ runDuration(data) }}</template>
                </Column>
                <Column header="Status">
                    <template #body="{ data }">
                        <Tag
                            :value="statusLabel(data.status)"
                            :severity="statusSeverity(data.status)"
                        />
                    </template>
                </Column>
                <Column header="Actions" style="width: 9rem">
                    <template #body="{ data }">
                        <div class="run-actions">
                            <Button
                                v-if="isActiveStatus(data.status)"
                                label="Cancel"
                                icon="pi pi-times"
                                size="small"
                                severity="secondary"
                                :loading="cancelingId === data.id"
                                :disabled="cancelingId != null"
                                :data-test="`cancel-${data.id}`"
                                @click.stop="emit('cancel', data.id)"
                            />
                            <Button
                                icon="pi pi-info-circle"
                                text
                                rounded
                                size="small"
                                severity="secondary"
                                aria-label="Run log"
                                :data-test="`log-${data.id}`"
                                @click.stop="emit('open', data as JobRun)"
                            />
                        </div>
                    </template>
                </Column>
            </DataTable>
        </div>
    </div>
</template>

<style scoped>
.runs-summary {
    margin: 0 0 0.5rem;
    font-size: 0.85rem;
    color: var(--app-text-muted);
}
.runs-table :deep(.c-datatable-tbody > tr) {
    cursor: pointer;
}
.run-actions {
    display: flex;
    align-items: center;
    gap: 0.25rem;
}
.empty-table {
    padding: 2rem;
    text-align: center;
    color: var(--app-text-muted);
}
.table-fit {
    overflow-x: auto;
}
/* a phone keeps job, status and actions; the log dialog is a tap away */
@media (max-width: 640px) {
    .table-fit :deep(.col-wide) {
        display: none;
    }
}
</style>
