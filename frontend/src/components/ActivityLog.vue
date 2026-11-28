<script setup lang="ts">
import { onMounted, ref, shallowRef } from "vue";

import { api, type Activity } from "../api/client";
import { apiError } from "../i18n/apiErrors";
import { formatBytes, formatDate, formatDateTime, t } from "../i18n";
import EmptyState from "./EmptyState.vue";

type ActivityEvent = Activity["events"][number];

const journal = shallowRef<Activity | null>(null);
const events = shallowRef<ActivityEvent[]>([]);
const next = ref<number | null>(null);
const loading = ref(false);
const error = ref("");

onMounted(() => load());

async function load(before?: number) {
  loading.value = true;
  const { data, error: failure } = await api.GET("/activity", {
    params: { query: { before } },
  });
  loading.value = false;
  if (!data) {
    error.value = failure ? apiError(failure) : "";
    return;
  }
  error.value = "";
  journal.value = data;
  events.value =
    before === undefined ? data.events : [...events.value, ...data.events];
  next.value = data.next;
}

function actionText(action: string) {
  return action === "login"
    ? t("activity.actions.login")
    : action === "upload"
      ? t("activity.actions.upload")
      : action;
}

function outcomeText(outcome: string) {
  switch (outcome) {
    case "succeeded":
      return t("activity.outcomes.succeeded");
    case "rejected":
      return t("activity.outcomes.rejected");
    case "invalid_request":
      return t("activity.outcomes.invalidRequest");
    case "error":
      return t("activity.outcomes.error");
    case "accepted":
      return t("activity.outcomes.accepted");
    case "duplicate":
      return t("activity.outcomes.duplicate");
    case "restored":
      return t("activity.outcomes.restored");
    default:
      return outcome;
  }
}

// The day comes as YYYY-MM-DD in Berlin; noon keeps it on that date in any zone.
function dayText(day: string) {
  return formatDate(`${day}T12:00:00`);
}

function failed(event: ActivityEvent) {
  return event.httpStatus >= 500;
}
</script>

<template>
  <div class="activity">
    <p class="text-medium-emphasis">{{ t("activity.description") }}</p>
    <div>
      <v-alert
        v-if="error"
        type="error"
        density="compact"
        class="mb-4"
        :title="error"
      />
      <v-alert
        v-if="journal && !journal.enabled"
        type="info"
        density="compact"
        class="mb-4"
        :title="t('activity.disabled')"
        data-testid="activity-disabled"
      />

      <EmptyState
        v-if="journal && journal.events.length === 0"
        icon="fa-clock-rotate-left"
        :title="t('activity.none')"
        :text="t('activity.noneText')"
      />

      <template v-else-if="journal">
        <h2>{{ t("activity.days") }}</h2>
        <v-table class="mb-6" data-testid="activity-days">
          <thead>
            <tr>
              <th>{{ t("activity.day") }}</th>
              <th class="text-right">{{ t("activity.logins") }}</th>
              <th class="text-right">{{ t("activity.rejectedLogins") }}</th>
              <th class="text-right">{{ t("activity.invalidLogins") }}</th>
              <th class="text-right">{{ t("activity.loginErrors") }}</th>
              <th class="text-right">{{ t("activity.newFiles") }}</th>
              <th class="text-right">{{ t("activity.duplicates") }}</th>
              <th class="text-right">{{ t("activity.restored") }}</th>
              <th class="text-right">{{ t("activity.rejectedUploads") }}</th>
              <th class="text-right">{{ t("activity.uploadErrors") }}</th>
              <th class="text-right">
                {{ t("activity.sessionsWithUploads") }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="day in journal.days" :key="day.day">
              <td>{{ dayText(day.day) }}</td>
              <td class="text-right">{{ day.logins }}</td>
              <td class="text-right">{{ day.rejectedLogins }}</td>
              <td class="text-right">{{ day.invalidLogins }}</td>
              <td class="text-right">{{ day.loginErrors }}</td>
              <td class="text-right">{{ day.newFiles }}</td>
              <td class="text-right">{{ day.duplicates }}</td>
              <td class="text-right">{{ day.restored }}</td>
              <td class="text-right">{{ day.rejectedUploads }}</td>
              <td class="text-right">{{ day.uploadErrors }}</td>
              <td class="text-right">{{ day.sessionsWithUploads }}</td>
            </tr>
          </tbody>
        </v-table>

        <h2>{{ t("activity.events") }}</h2>
        <v-table data-testid="activity-events">
          <thead>
            <tr>
              <th>{{ t("activity.time") }}</th>
              <th>{{ t("activity.action") }}</th>
              <th>{{ t("activity.outcome") }}</th>
              <th class="text-right">{{ t("activity.status") }}</th>
              <th>{{ t("activity.client") }}</th>
              <th>{{ t("activity.session") }}</th>
              <th>{{ t("activity.file") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="event in events" :key="event.id">
              <td>{{ formatDateTime(event.occurredAt) }}</td>
              <td>{{ actionText(event.action) }}</td>
              <td :class="{ 'text-error': failed(event) }">
                {{ outcomeText(event.outcome) }}
              </td>
              <td class="text-right" :class="{ 'text-error': failed(event) }">
                {{ event.httpStatus }}
              </td>
              <td>{{ event.browser }}, {{ event.os }}</td>
              <td>
                <code v-if="event.sessionId" :title="event.sessionId">{{
                  event.sessionId.slice(0, 8)
                }}</code>
              </td>
              <td>
                <template v-if="event.mediaType">
                  {{ event.mediaType
                  }}<span v-if="event.sizeBytes !== null"
                    >, {{ formatBytes(event.sizeBytes) }}</span
                  >
                </template>
              </td>
            </tr>
          </tbody>
        </v-table>
        <div v-if="next !== null" class="mt-4">
          <v-btn
            :text="t('activity.older')"
            :loading="loading"
            @click="load(next!)"
          />
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.activity {
  max-width: 1200px;
}
</style>
