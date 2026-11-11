<script setup lang="ts">
import { computed, ref } from 'vue'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useToast } from 'primevue/usetoast'
import JobRunsTable from '@/components/jobs/JobRunsTable.vue'
import JobRunLogDialog from '@/components/jobs/JobRunLogDialog.vue'
import JobScheduleDialog from '@/components/jobs/JobScheduleDialog.vue'
import { useCancelJobRun, useJobRuns, useJobs, useRunJob } from '@/composables/useApi'
import { apiErrorMessage } from '@/lib/apiError'
import type { JobRun } from '@/lib/api/types'
import { formatDate } from '@/lib/format'
import {
    isActiveStatus,
    jobRows,
    progressLabel,
    scheduleSummary,
    statusLabel,
    type JobRow
} from '@/lib/jobs'

// The background jobs of the tenant, after aether's Tasks page: the Jobs tab
// runs a job now or schedules it; the Runs tab lists the tenant's runs, each
// with its log, and cancels the ones still going. The runs are polled while one
// is waiting or running, so the Run button follows its progress.
const toast = useToast()
const activeTab = ref('jobs')

const jobsQuery = useJobs()
const runsQuery = useJobRuns()
const runJob = useRunJob()
const cancelRun = useCancelJobRun()

const jobs = computed(() => jobsQuery.data.value ?? [])
const runs = computed(() => runsQuery.data.value ?? [])
const rows = computed<JobRow[]>(() => jobRows(jobs.value, runs.value))

const loadError = computed(() => {
    if (jobsQuery.isError.value) return apiErrorMessage(jobsQuery.error.value)
    if (runsQuery.isError.value) return apiErrorMessage(runsQuery.error.value)
    return ''
})

const isRunning = (row: JobRow) => isActiveStatus(row.lastRun?.status)

// the job whose Run request is in flight
const startingJob = ref<string | null>(null)

async function run(row: JobRow) {
    startingJob.value = row.name
    try {
        const { reused } = await runJob.mutateAsync(row.name)
        if (reused) {
            toast.add({
                severity: 'info',
                summary: 'Already in progress',
                detail: `${row.title} is already queued or running.`,
                life: 4000
            })
        }
    } catch (err) {
        toast.add({
            severity: 'error',
            summary: `${row.title} could not start`,
            detail: apiErrorMessage(err),
            life: 5000
        })
    } finally {
        startingJob.value = null
    }
}

const cancelingId = ref<string | null>(null)

async function cancel(runId: string) {
    cancelingId.value = runId
    try {
        await cancelRun.mutateAsync(runId)
    } catch (err) {
        toast.add({
            severity: 'error',
            summary: 'The run could not be canceled',
            detail: apiErrorMessage(err),
            life: 5000
        })
    } finally {
        cancelingId.value = null
    }
}

// The schedule dialog tracks the job's name and looks the job up in the query,
// so it shows the refetched schedules after every change.
const scheduleVisible = ref(false)
const scheduleJobName = ref<string | null>(null)
const scheduleJob = computed(() => jobs.value.find((j) => j.name === scheduleJobName.value) ?? null)

function openSchedules(row: JobRow) {
    scheduleJobName.value = row.name
    scheduleVisible.value = true
}

// The log dialog follows the run in the polled list, so its status stays live.
const logVisible = ref(false)
const logRunId = ref<string | null>(null)
const logRun = computed(() => runs.value.find((r) => r.id === logRunId.value) ?? null)
const logTitle = computed(
    () => jobs.value.find((j) => j.name === logRun.value?.job)?.title ?? logRun.value?.job
)

function openLog(r: JobRun) {
    logRunId.value = r.id
    logVisible.value = true
}

function lastRunText(row: JobRow): string {
    const r = row.lastRun
    if (!r) return 'Never run'
    return `${statusLabel(r.status)} · ${formatDate(r.startedAt ?? r.queuedAt)}`
}
</script>

<template>
    <section aria-label="Jobs">
        <h2 class="set-section-title">Jobs</h2>
        <p class="set-note">
            Background work on your tenant's notes. Run a job now or on a schedule, and follow its
            runs and logs.
        </p>

        <Message v-if="loadError" severity="error" :closable="false" class="load-error">
            {{ loadError }}
        </Message>

        <Tabs v-model:value="activeTab">
            <TabList>
                <Tab value="jobs" data-test="tab-jobs">Jobs</Tab>
                <Tab value="runs" data-test="tab-runs">Runs</Tab>
            </TabList>
            <TabPanels>
                <TabPanel value="jobs">
                    <div class="table-fit">
                        <DataTable
                            :value="rows"
                            :loading="jobsQuery.isLoading.value"
                            dataKey="name"
                            size="small"
                        >
                            <template #empty><div class="empty-table">No jobs</div></template>
                            <Column header="Job">
                                <template #body="{ data }">
                                    <div :data-test="`job-${data.name}`">
                                        <span class="job-title">{{ data.title }}</span>
                                        <p class="job-desc">{{ data.description }}</p>
                                        <p
                                            v-if="isRunning(data) && data.lastRun?.progress?.stage"
                                            class="job-stage"
                                            :data-test="`stage-${data.name}`"
                                        >
                                            {{ data.lastRun.progress.stage }}
                                        </p>
                                        <p v-else class="job-last">{{ lastRunText(data) }}</p>
                                    </div>
                                </template>
                            </Column>
                            <Column header="Schedule" headerClass="col-wide" bodyClass="col-wide">
                                <template #body="{ data }">
                                    <span
                                        class="schedule-summary"
                                        :data-test="`schedule-summary-${data.name}`"
                                        >{{ scheduleSummary(data.schedules) }}</span
                                    >
                                </template>
                            </Column>
                            <Column style="width: 3.5rem">
                                <template #body="{ data }">
                                    <Button
                                        icon="pi pi-calendar"
                                        text
                                        rounded
                                        size="small"
                                        :aria-label="
                                            data.schedules.length ? 'Edit schedules' : 'Schedule'
                                        "
                                        :data-test="`schedule-${data.name}`"
                                        @click="openSchedules(data)"
                                    />
                                </template>
                            </Column>
                            <Column header="Actions" style="width: 10rem">
                                <template #body="{ data }">
                                    <Button
                                        :label="
                                            isRunning(data)
                                                ? progressLabel(
                                                      data.lastRun?.status,
                                                      data.lastRun?.progress
                                                  )
                                                : 'Run now'
                                        "
                                        :icon="isRunning(data) ? undefined : 'pi pi-play'"
                                        size="small"
                                        outlined
                                        :loading="startingJob === data.name || isRunning(data)"
                                        :disabled="startingJob !== null || isRunning(data)"
                                        :data-test="`run-${data.name}`"
                                        @click="run(data)"
                                    />
                                </template>
                            </Column>
                        </DataTable>
                    </div>
                </TabPanel>

                <TabPanel value="runs">
                    <JobRunsTable
                        :runs="runs"
                        :jobs="jobs"
                        :loading="runsQuery.isLoading.value"
                        :cancelingId="cancelingId"
                        @cancel="cancel"
                        @open="openLog"
                    />
                </TabPanel>
            </TabPanels>
        </Tabs>

        <JobScheduleDialog v-model:visible="scheduleVisible" :job="scheduleJob" />
        <JobRunLogDialog v-model:visible="logVisible" :run="logRun" :title="logTitle" />
    </section>
</template>

<style scoped>
.load-error {
    margin-bottom: 1rem;
}
.job-title {
    font-size: 0.9rem;
    font-weight: 600;
}
.job-desc,
.job-last,
.job-stage {
    margin: 0.2rem 0 0;
    font-size: 0.8rem;
    color: var(--app-text-muted);
}
.job-stage {
    font-style: italic;
}
.schedule-summary {
    font-size: 0.85rem;
    color: var(--app-text-muted);
}
.empty-table {
    padding: 2rem;
    text-align: center;
    color: var(--app-text-muted);
}
.table-fit {
    overflow-x: auto;
}
@media (max-width: 640px) {
    .table-fit :deep(.col-wide) {
        display: none;
    }
}
</style>
