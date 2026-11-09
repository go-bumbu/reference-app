<script setup lang="ts">
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'
import SettingsRow from '@/components/settings/SettingsRow.vue'

// What runs in the background, after codex's Jobs page: each job with a Run now.
//
// PLACEHOLDER: the server has no background workers yet (docs/agents/features.md),
// so the jobs are a fixed list and Run now only says so. When jobs land, list
// them from the API with a TanStack query, start one with a mutation, and poll
// the runs while one is active.
const PLACEHOLDER_JOBS = [
    {
        key: 'reindex-notes',
        name: 'Rebuild search index',
        desc: 'Re-reads every note of the tenant into the search index.'
    },
    {
        key: 'export-notes',
        name: 'Export notes',
        desc: 'Writes every note of the tenant to a downloadable archive.'
    },
    {
        key: 'cleanup',
        name: 'Clean up',
        desc: 'Removes leftovers of deleted notes.'
    }
]

const toast = useToast()

function run(name: string) {
    toast.add({
        severity: 'info',
        summary: `${name}: jobs are not implemented yet`,
        detail: 'The server has no background workers yet.',
        life: 4000
    })
}
</script>

<template>
    <section aria-label="Jobs">
        <h2 class="set-section-title">Jobs</h2>
        <p class="set-note">
            Background work of your tenant. <Tag value="Placeholder" severity="secondary" /> These
            jobs are samples: running one does nothing yet.
        </p>
        <div class="set-card">
            <SettingsRow
                v-for="job in PLACEHOLDER_JOBS"
                :key="job.key"
                :name="job.name"
                :desc="job.desc"
                :data-test="`job-${job.key}`"
            >
                <div class="job-run">
                    <span class="job-last muted">Never run</span>
                    <Button
                        label="Run now"
                        icon="pi pi-play"
                        size="small"
                        outlined
                        :data-test="`run-${job.key}`"
                        @click="run(job.name)"
                    />
                </div>
            </SettingsRow>
        </div>
    </section>
</template>

<style scoped>
.job-run {
    display: flex;
    align-items: center;
    gap: 0.75rem;
}

.job-last {
    font-size: 0.8rem;
}
</style>
