<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { api } from "../api";
type State = "failed" | "running" | "pending";
interface Item {
  id: number;
  filename: string;
  folder: string;
  library: string;
  stage: string;
  metadata: string;
  preview: string;
  error: string;
  attempts: number;
  nextAt: number;
  trashed: boolean;
}
interface Snapshot {
  total: number;
  metadataReady: number;
  previewsReady: number;
  counts: Record<State, number>;
  items: Item[];
  scanRunning: boolean;
  scanExamined: number;
  lastScan: string;
  scanError: string;
  serverTime: number;
}
const emit = defineEmits<{ changed: [] }>();
const data = ref<Snapshot>();
const storage = ref<{
  cache: {
    bytes: number;
    removedBytes: number;
    requeued: number;
    checkedAt: string;
    error: string;
  };
  uploadedBytes: number;
  trashBytes: number;
}>();
const cleaning = ref(false);
function size(bytes: number) {
  return `${(bytes / 1024 / 1024).toLocaleString(undefined, { maximumFractionDigits: 1 })} MiB`;
}
async function cleanCache() {
  cleaning.value = true;
  try {
    storage.value = await api("/cache", "POST");
    notice.value = `Freed ${size(storage.value!.cache.removedBytes)}. ${storage.value!.cache.requeued} missing previews queued.`;
    emit("changed");
  } catch (e) {
    error.value = String(e);
  } finally {
    cleaning.value = false;
  }
}
const state = ref<State>("failed"),
  offset = ref(0),
  error = ref(""),
  notice = ref(""),
  busy = ref(false),
  loading = ref(true),
  confirmRebuild = ref(false);
let timer: ReturnType<typeof setTimeout> | undefined;
let controller: AbortController | undefined;
let disposed = false;
async function refresh() {
  clearTimeout(timer);
  controller?.abort();
  const current = new AbortController();
  controller = current;
  loading.value = true;
  try {
    const result = await api<Snapshot>(
      `/processing?state=${state.value}&offset=${offset.value}`,
      "GET",
      undefined,
      current.signal,
    );
    if (current.signal.aborted) return;
    data.value = result;
    storage.value = await api("/cache", "GET", undefined, current.signal);
    error.value = "";
    if (!result.items.length && offset.value > 0) {
      offset.value = 0;
      void refresh();
      return;
    }
  } catch (e) {
    if (!current.signal.aborted) error.value = String(e);
  } finally {
    if (!disposed && controller === current) {
      loading.value = false;
      timer = setTimeout(() => {
        if (!document.hidden) void refresh();
        else schedule();
      }, 3000);
    }
  }
}
function schedule() {
  timer = setTimeout(() => {
    if (!document.hidden) void refresh();
    else schedule();
  }, 3000);
}
function select(value: State) {
  state.value = value;
  offset.value = 0;
  void refresh();
}
function page(delta: number) {
  offset.value += delta;
  void refresh();
}
async function action(kind: "scan" | "retry" | "rebuild", id = 0) {
  busy.value = true;
  notice.value = "";
  try {
    if (kind === "retry") {
      const result = await api<{ retried: number }>(
        "/processing/retry",
        "POST",
        { id },
      );
      notice.value = result.retried
        ? `${result.retried} failed file${result.retried === 1 ? "" : "s"} queued for another attempt.`
        : "No failed jobs remain to retry.";
    } else {
      await api(kind === "scan" ? "/scan" : "/retry", "POST");
      notice.value =
        kind === "scan"
          ? "Scan requested. Progress appears when discovery starts."
          : "Preview rebuild queued. Originals stay unchanged.";
    }
    confirmRebuild.value = false;
    emit("changed");
    await refresh();
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
function date(value: string) {
  return new Date(value).toLocaleString();
}
function waiting(item: Item) {
  return data.value && item.nextAt > data.value.serverTime
    ? `Automatic retry in about ${Math.ceil((item.nextAt - data.value.serverTime) / 60)} min`
    : "Waiting for a worker";
}
onMounted(() => void refresh());
onUnmounted(() => {
  disposed = true;
  clearTimeout(timer);
  controller?.abort();
});
</script>

<template>
  <section class="processing-dashboard" aria-label="Library processing">
    <p class="scope-note">
      All enabled libraries, including files in Trash. Updates every 3 seconds
      while this page is visible.
    </p>
    <p v-if="error" role="alert" class="error">
      {{ error }} <button @click="refresh">Refresh status</button>
    </p>
    <p v-if="notice" role="status">{{ notice }}</p>
    <p v-if="!data && loading" role="status">Loading processing status…</p>
    <template v-if="data">
      <section class="scan-panel">
        <div>
          <p class="section-label">LIBRARY DISCOVERY</p>
          <h2>
            {{
              data.scanRunning
                ? "Scanning your folders…"
                : data.scanError
                  ? "Scan needs attention"
                  : "Ready to scan"
            }}
          </h2>
          <p v-if="data.scanRunning">
            {{ data.scanExamined.toLocaleString() }} supported files examined.
            The total is known after discovery finishes.
          </p>
          <p v-else-if="data.lastScan">
            Last successful scan: {{ date(data.lastScan) }}
          </p>
          <p v-else>No completed scan recorded since this server started.</p>
          <p v-if="data.scanError" class="error scan-error">
            {{ data.scanError }}
          </p>
          <small
            >External folders are scanned periodically. Uploads enter the
            processing queue immediately.</small
          >
        </div>
        <button
          class="primary"
          :disabled="busy || data.scanRunning"
          @click="action('scan')"
        >
          Scan library
        </button>
      </section>
      <div class="processing-metrics">
        <article>
          <span>Indexed originals</span
          ><strong>{{ data.total.toLocaleString() }}</strong
          ><small>Files known to the database</small>
        </article>
        <article>
          <span>Metadata ready</span
          ><strong
            >{{ data.metadataReady.toLocaleString() }}
            <small>/ {{ data.total.toLocaleString() }}</small></strong
          ><progress
            :value="data.metadataReady"
            :max="data.total || 1"
            aria-label="Files with metadata ready"
          /><small>Dates, camera details and available locations</small>
        </article>
        <article>
          <span>Previews ready</span
          ><strong
            >{{ data.previewsReady.toLocaleString() }}
            <small>/ {{ data.total.toLocaleString() }}</small></strong
          ><progress
            :value="data.previewsReady"
            :max="data.total || 1"
            aria-label="Files with previews ready"
          /><small
            >Generated preview records; cache files are not audited here</small
          >
        </article>
      </div>
      <section v-if="storage" class="maintenance">
        <h2>Storage &amp; cache</h2>
        <p>
          Uploaded originals: {{ size(storage.uploadedBytes) }} · In Trash:
          {{ size(storage.trashBytes) }} · Cache:
          {{ size(storage.cache.bytes) }}
        </p>
        <p class="scope-note">
          Checked hourly. Unused generated files are removed after 24 hours;
          full-size image and video copies expire after 30 days without use.
          Missing previews are rebuilt. Originals and Trash are kept until you
          delete them.
        </p>
        <p v-if="storage.cache.checkedAt">
          <small>Last check: {{ date(storage.cache.checkedAt) }}</small>
        </p>
        <p v-if="storage.cache.error" class="error">
          {{ storage.cache.error }}
        </p>
        <button :disabled="cleaning" @click="cleanCache">
          {{ cleaning ? "Checking…" : "Check & clean cache" }}
        </button>
      </section>
      <section class="jobs-panel">
        <div class="queue-heading">
          <h2>Processing queue</h2>
          <button
            :disabled="busy || !data.counts.failed"
            @click="action('retry')"
          >
            Retry failed ({{ data.counts.failed }})
          </button>
        </div>
        <div
          class="queue-filters"
          role="group"
          aria-label="Filter processing jobs"
        >
          <button
            v-for="tab in [
              ['failed', 'Needs attention'],
              ['running', 'Processing now'],
              ['pending', 'Waiting'],
            ] as const"
            :key="tab[0]"
            :aria-pressed="state === tab[0]"
            :class="{ selected: state === tab[0] }"
            @click="select(tab[0])"
          >
            {{ tab[1] }} <span>{{ data.counts[tab[0]] }}</span>
          </button>
        </div>
        <p class="scope-note">
          Metadata and previews share one job per file. Failed attempts retry
          automatically up to three times.
        </p>
        <div v-if="!data.items.length" class="queue-empty">
          {{
            state === "failed"
              ? "No files need attention."
              : state === "running"
                ? "No files are processing right now."
                : "No files are waiting."
          }}
        </div>
        <ul v-else class="job-list">
          <li v-for="item in data.items" :key="item.id">
            <div class="job-name">
              <strong>{{ item.filename }}</strong
              ><small
                >{{ item.library }} / {{ item.folder || "Library root" }} · #{{
                  item.id
                }}{{ item.trashed ? " · In Trash" : "" }}</small
              >
            </div>
            <p class="job-state">
              {{
                state === "running"
                  ? item.stage === "previews"
                    ? "Generating previews"
                    : "Extracting metadata"
                  : state === "pending"
                    ? waiting(item)
                    : "Needs attention"
              }}
              · {{ item.attempts }} failed attempt{{
                item.attempts === 1 ? "" : "s"
              }}
            </p>
            <small
              >Last result: metadata {{ item.metadata }} · previews
              {{ item.preview }}</small
            >
            <details v-if="item.error">
              <summary>
                {{
                  state === "failed"
                    ? "Processing error"
                    : "Previous attempt error"
                }}
              </summary>
              <pre>{{ item.error }}</pre>
            </details>
            <button
              v-if="state === 'failed'"
              :disabled="busy"
              @click="action('retry', item.id)"
            >
              Retry this file
            </button>
          </li>
        </ul>
        <div v-if="offset > 0 || data.counts[state] > 50" class="queue-pages">
          <button :disabled="loading || offset === 0" @click="page(-50)">
            Previous</button
          ><span
            >{{ offset + 1 }}–{{ offset + data.items.length }} of
            {{ data.counts[state] }}</span
          ><button
            :disabled="loading || offset + 50 >= data.counts[state]"
            @click="page(50)"
          >
            Next
          </button>
        </div>
      </section>
      <details class="maintenance">
        <summary>Preview maintenance</summary>
        <p>
          Rebuild generated previews after replacing the cache. This also
          retries failed jobs and can take a long time for large libraries.
        </p>
        <button v-if="!confirmRebuild" @click="confirmRebuild = true">
          Rebuild previews…
        </button>
        <div v-else>
          <p>Queue a rebuild of existing previews across the library?</p>
          <button :disabled="busy" @click="action('rebuild')">
            Start rebuild
          </button>
          <button :disabled="busy" @click="confirmRebuild = false">
            Cancel
          </button>
        </div>
      </details>
    </template>
  </section>
</template>

<style scoped>
.processing-dashboard {
  display: grid;
  gap: 24px;
  max-width: 1200px;
}
.scope-note,
small {
  color: var(--muted);
}
.scope-note {
  margin: 0;
  font-size: 13px;
}
.scan-panel,
.jobs-panel,
.maintenance,
.processing-metrics article {
  border: 1px solid var(--line);
  border-radius: var(--radius-panel);
  background: var(--surface);
  padding: 24px;
}
.scan-panel,
.queue-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 20px;
}
h2 {
  font-size: 21px;
  margin: 0 0 8px;
}
.section-label {
  font-size: 11px;
  letter-spacing: 0.12em;
  color: var(--muted);
  margin: 0 0 12px;
}
.scan-panel button {
  flex-shrink: 0;
}
.scan-error {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.processing-metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}
.processing-metrics article {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.processing-metrics strong {
  font-size: 32px;
  font-weight: 550;
}
.processing-metrics strong small {
  font-size: 15px;
}
progress {
  width: 100%;
  height: 7px;
  accent-color: var(--accent);
}
.queue-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 20px 0 12px;
}
button {
  border: 1px solid var(--line);
  padding: 10px 14px;
  border-radius: var(--radius-control);
}
.selected {
  background: var(--active);
  color: var(--accent);
  border-color: var(--accent);
}
.queue-filters span {
  margin-left: 8px;
}
.job-list {
  list-style: none;
  padding: 0;
  margin: 20px 0 0;
}
.job-list li {
  padding: 20px 0;
  border-top: 1px solid var(--line);
  display: grid;
  gap: 10px;
}
.job-name {
  display: grid;
  gap: 6px;
  overflow-wrap: anywhere;
}
.job-state {
  margin: 0;
}
.job-list button {
  justify-self: start;
}
summary {
  cursor: pointer;
}
pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font: inherit;
  font-size: 13px;
  color: var(--muted);
  background: var(--bg);
  padding: 14px;
  border-radius: var(--radius-control);
}
.queue-empty {
  padding: 36px 0 20px;
  color: var(--muted);
}
.queue-pages {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-top: 16px;
}
@media (max-width: 760px) {
  .processing-metrics {
    grid-template-columns: 1fr;
  }
  .scan-panel,
  .queue-heading {
    align-items: flex-start;
    flex-direction: column;
  }
  .scan-panel,
  .jobs-panel,
  .maintenance,
  .processing-metrics article {
    padding: 18px;
  }
}
</style>
