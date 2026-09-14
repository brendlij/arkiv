<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { hashFile } from "../sha256";
import { api, ApiError } from "../api";
const props = defineProps<{ folder?: string }>();
const destination = ref("");
const destinations = ref<string[]>([]);
type UploadOptions = {
  formats: { extension: string; kind: string; mime: string }[];
  imageLimit: number;
  videoLimit: number;
  quota: number;
  enabled: boolean;
  used: number;
  reserved: number;
  userId: number;
  chunkSize: number;
  retentionHours: number;
};
const options = ref<UploadOptions>(),
  optionsError = ref(""),
  optionsLoading = ref(false),
  selectionNotice = ref("");
const accept = computed(
  () => options.value?.formats.map((f) => f.extension).join(",") || "",
);
const formatGroups = computed(() =>
  ["image", "raw", "video"].map((kind) => ({
    name:
      kind === "image" ? "Photos" : kind === "raw" ? "Camera RAW" : "Videos",
    extensions: options.value?.formats
      .filter((f) => f.kind === kind)
      .map((f) => f.extension.slice(1).toUpperCase())
      .join(", "),
  })),
);
async function open() {
  if (!items.value.length) destination.value = props.folder || "";
  dialog.value?.showModal();
  optionsLoading.value = true;
  optionsError.value = "";
  try {
    options.value = await api<UploadOptions>("/uploads");
    destinations.value = await api<string[]>("/me/upload-folders");
    await refreshPending();
    if (!options.value.enabled)
      optionsError.value = "Uploads are not configured on this server.";
  } catch (e) {
    optionsError.value = (e as Error).message;
  } finally {
    optionsLoading.value = false;
  }
}
const emit = defineEmits<{
  saved: [];
  completed: [count: number, folder: string];
}>();
const dialog = ref<HTMLDialogElement>();
const picker = ref<HTMLInputElement>();
const dragging = ref(false);
const busy = ref(false);

type Session = {
  id: string;
  filename: string;
  folder: string;
  size: number;
  sha256: string;
  keepCopy: boolean;
  received: number;
  state: string;
  assetId: number;
  trashed: boolean;
  updatedAt: number;
};
type Item = {
  file: File;
  progress: number;
  status: string;
  error: string;
  hash?: string;
  session?: Session;
};
const items = ref<Item[]>([]),
  pending = ref<Session[]>([]),
  keepCopies = ref(false),
  requestedResume = ref<Session>();
let active: XMLHttpRequest | undefined;
let activeRequest: AbortController | undefined;
let cancelled = false;
function bytes(n: number) {
  return `${(n / 1073741824).toFixed(2)} GiB`;
}
function stop() {
  cancelled = true;
  active?.abort();
  activeRequest?.abort();
}
onUnmounted(stop);
async function refreshPending() {
  pending.value = await api<Session[]>("/uploads/sessions");
}
function add(files: FileList | null) {
  if (!files || busy.value) return;
  selectionNotice.value =
    files.length > 50 ? "Only the first 50 files were added." : "";
  items.value = Array.from(files)
    .slice(0, 50)
    .map((file) => ({ file, progress: 0, status: "Ready", error: "" }));
}
function resume(session: Session) {
  requestedResume.value = session;
  destination.value = session.folder.replace(/^user-\d+\/?/, "");
  keepCopies.value = session.keepCopy;
  picker.value?.click();
}
async function discard(session: Session) {
  if (busy.value) return;
  busy.value = true;
  optionsError.value = "";
  try {
    await api(`/uploads/sessions/${session.id}`, "DELETE");
    for (const item of items.value)
      if (item.session?.id === session.id) {
        item.session = undefined;
        item.status = "Ready";
      }
    if (requestedResume.value?.id === session.id)
      requestedResume.value = undefined;
    await refreshPending();
    options.value = await api<UploadOptions>("/uploads");
  } catch (e) {
    optionsError.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function finishPending(session: Session) {
  if (busy.value) return;
  busy.value = true;
  cancelled = false;
  optionsError.value = "";
  try {
    const result = await request<Session>(
      `/uploads/sessions/${session.id}/complete`,
      "POST",
    );
    selectionNotice.value =
      result.state === "duplicate"
        ? `Already uploaded${result.trashed ? " (in Trash)" : ""}.`
        : "Upload recovered and saved.";
    emit("saved");
    await refreshPending();
    options.value = await api<UploadOptions>("/uploads");
  } catch (e) {
    optionsError.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
watch(keepCopies, () => {
  for (const item of items.value)
    if (item.status === "Already uploaded") {
      item.status = "Ready";
      item.session = undefined;
      item.error = "";
    }
});
async function request<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    if (cancelled) throw new Error("Paused");
    const controller = new AbortController();
    activeRequest = controller;
    const timeout = setTimeout(() => controller.abort(), 125000);
    try {
      return await api<T>(path, method, body, controller.signal);
    } catch (e) {
      const retry =
        !(e instanceof ApiError) || e.status === 429 || e.status >= 500;
      if (!retry || attempt >= 3 || cancelled) throw e;
      await new Promise((resolve) => setTimeout(resolve, 1000 * 2 ** attempt));
    } finally {
      clearTimeout(timeout);
      activeRequest = undefined;
    }
  }
}
function cacheKey(item: Item) {
  return `arkiv-upload:${options.value!.userId}:${item.hash}:${item.file.size}:${item.file.name}:${destination.value}:${keepCopies.value}`;
}
function readReceipt(
  key: string,
): { clientKey: string; id?: string } | undefined {
  try {
    return JSON.parse(localStorage.getItem(key) || "null") || undefined;
  } catch {
    return undefined;
  }
}
function writeReceipt(key: string, value: unknown) {
  try {
    if (value) localStorage.setItem(key, JSON.stringify(value));
    else localStorage.removeItem(key);
  } catch {
    /* Server-side sessions remain resumable if browser storage is unavailable. */
  }
}
function newKey() {
  const b = new Uint8Array(16);
  crypto.getRandomValues(b);
  return Array.from(b, (v) => v.toString(16).padStart(2, "0")).join("");
}
function chunk(item: Item, session: Session) {
  return new Promise<Session>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    active = xhr;
    xhr.open("PATCH", `/api/uploads/sessions/${session.id}`);
    xhr.setRequestHeader("X-Gallery-Request", "1");
    xhr.setRequestHeader("Upload-Offset", String(session.received));
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    xhr.timeout = 125000;
    xhr.upload.onprogress = (e) => {
      item.progress = Math.round(
        ((session.received + e.loaded) / item.file.size) * 100,
      );
    };
    xhr.onload = () => {
      active = undefined;
      if (xhr.status === 200) {
        try {
          resolve(JSON.parse(xhr.responseText));
        } catch {
          reject(
            new Error(
              "Could not read server acknowledgment. Resume to check progress.",
            ),
          );
        }
      } else
        reject(
          new ApiError(xhr.status, xhr.responseText || "Chunk upload failed"),
        );
    };
    xhr.onerror = xhr.ontimeout = () => {
      active = undefined;
      reject(new Error("Connection interrupted"));
    };
    xhr.onabort = () => {
      active = undefined;
      reject(new Error("Paused"));
    };
    xhr.send(
      item.file.slice(
        session.received,
        session.received + options.value!.chunkSize,
      ),
    );
  });
}
async function upload() {
  if (busy.value || !options.value?.enabled) return;
  busy.value = true;
  cancelled = false;
  optionsError.value = "";
  for (const item of items.value) {
    if (cancelled) break;
    if (item.status === "Saved" || item.status === "Already uploaded") continue;
    item.error = "";
    try {
      const format = options.value.formats.find(
        (f) =>
          f.extension === "." + item.file.name.split(".").pop()?.toLowerCase(),
      );
      if (
        !format ||
        item.file.size <= 0 ||
        item.file.size >
          (format.kind === "video"
            ? options.value.videoLimit
            : options.value.imageLimit)
      )
        throw new Error(
          "Unsupported format or file size. See the limits below.",
        );
      if (!item.hash) {
        item.status = "Checking file";
        item.progress = 0;
        item.hash = await hashFile(
          item.file,
          (v) => (item.progress = v),
          () => cancelled,
        );
      }
      if (
        requestedResume.value &&
        (item.hash !== requestedResume.value.sha256 ||
          item.file.size !== requestedResume.value.size)
      )
        throw new Error(
          "This is not the original file for the paused upload. Choose that original file or discard the paused upload.",
        );
      item.status = "Checking upload";
      const key = cacheKey(item);
      let receipt = readReceipt(key) || { clientKey: newKey() };
      writeReceipt(key, receipt);
      let session = item.session || requestedResume.value;
      if (session) {
        try {
          session = await request<Session>(`/uploads/sessions/${session.id}`);
        } catch (e) {
          if (e instanceof ApiError && (e.status === 404 || e.status === 410)) {
            session = undefined;
            item.session = undefined;
            requestedResume.value = undefined;
            receipt = { clientKey: newKey() };
            writeReceipt(key, receipt);
          } else throw e;
        }
      }
      if (!session && receipt.id) {
        try {
          session = await request<Session>(`/uploads/sessions/${receipt.id}`);
        } catch (e) {
          if (e instanceof ApiError && (e.status === 404 || e.status === 410)) {
            receipt = { clientKey: newKey() };
            writeReceipt(key, receipt);
          } else throw e;
        }
      }
      if (!session)
        session = pending.value.find(
          (p) =>
            p.sha256 === item.hash &&
            p.size === item.file.size &&
            p.folder ===
              `user-${options.value!.userId}` +
                (destination.value ? "/" + destination.value : "") &&
            p.keepCopy === keepCopies.value,
        );
      if (!session)
        session = await request<Session>("/uploads/sessions", "POST", {
          filename: item.file.name,
          size: item.file.size,
          sha256: item.hash,
          folder: destination.value,
          keepCopy: keepCopies.value,
          clientKey: receipt.clientKey,
        });
      item.session = session;
      requestedResume.value = undefined;
      writeReceipt(key, { ...receipt, id: session.id });
      let failures = 0;
      while (
        session.state === "uploading" &&
        session.received < item.file.size
      ) {
        if (cancelled) throw new Error("Paused");
        item.status = "Uploading";
        item.progress = Math.round((session.received / item.file.size) * 100);
        try {
          session = await chunk(item, session);
          item.session = session;
          failures = 0;
        } catch (e) {
          if (cancelled) throw e;
          if (
            e instanceof ApiError &&
            e.status !== 409 &&
            e.status !== 429 &&
            e.status < 500
          )
            throw e;
          if (++failures > 3)
            throw new Error(
              "Connection interrupted. Resume to continue from the last saved chunk.",
            );
          item.status = "Reconnecting";
          await new Promise((resolve) =>
            setTimeout(resolve, 1000 * 2 ** (failures - 1)),
          );
          session = await request<Session>(`/uploads/sessions/${session.id}`);
          item.session = session;
        }
      }
      if (session.state !== "done" && session.state !== "duplicate") {
        item.status = "Verifying and saving";
        session = await request<Session>(
          `/uploads/sessions/${session.id}/complete`,
          "POST",
        );
      }
      item.session = session;
      item.status =
        session.state === "duplicate" ? "Already uploaded" : "Saved";
      item.progress = 100;
      item.error =
        session.state === "duplicate"
          ? session.trashed
            ? "The identical original is in Trash. Restore it there or choose Keep separate copies."
            : "An identical original already exists in your library; no extra copy was saved."
          : "";
      writeReceipt(key, null);
      emit("saved");
    } catch (e) {
      item.status = cancelled ? "Paused" : "Needs attention";
      item.error = (e as Error).message;
    }
  }
  active = undefined;
  busy.value = false;
  try {
    await refreshPending();
    options.value = await api<UploadOptions>("/uploads");
  } catch {
    /* Keep transfer results visible if the connection is still unavailable. */
  }
  if (
    !cancelled &&
    items.value.length &&
    items.value.every((item) => item.status === "Saved")
  ) {
    const count = items.value.length;
    dialog.value?.close();
    items.value = [];
    if (picker.value) picker.value.value = "";
    emit("completed", count, destination.value);
  }
}
</script>
<template>
  <button class="primary upload-trigger" @click="open">Upload</button>
  <Teleport to="body"
    ><dialog
      ref="dialog"
      class="upload-dialog"
      aria-labelledby="upload-title"
      @cancel="stop"
      @close="stop"
    >
      <header>
        <h2 id="upload-title">Add photos & videos</h2>
        <button
          class="icon-button"
          aria-label="Close upload"
          @click="dialog?.close()"
        >
          ×
        </button>
      </header>
      <button
        class="upload-drop"
        :class="{ dragging }"
        :disabled="busy || optionsLoading || !!optionsError"
        @click="picker?.click()"
        @dragover.prevent="dragging = true"
        @dragleave="dragging = false"
        @drop.prevent="
          dragging = false;
          add($event.dataTransfer?.files || null);
        "
      >
        <svg
          width="28"
          height="28"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
          aria-hidden="true"
        >
          <path
            d="M12 16V3m-5 5 5-5 5 5M4 15v5a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-5"
          />
        </svg>
        <strong>{{
          optionsLoading
            ? "Getting ready…"
            : items.length
              ? "Choose different files"
              : "Choose photos & videos"
        }}</strong>
        <span>or drag them here</span>
      </button>
      <input
        ref="picker"
        type="file"
        :accept="accept"
        multiple
        hidden
        @change="add(($event.target as HTMLInputElement).files)"
      />
      <p class="upload-help">
        Up to 50 files · Photos up to 250 MiB · Videos up to 2 GiB
      </p>
      <div class="upload-destination">
        <label for="upload-folder">Save to folder</label>
        <input
          id="upload-folder"
          v-model="destination"
          list="upload-folder-options"
          :disabled="busy || items.some((i) => !!i.session)"
          placeholder="My uploads"
          maxlength="240"
        />
        <datalist id="upload-folder-options">
          <option
            v-for="folder in destinations"
            :key="folder"
            :value="folder"
          />
        </datalist>
        <small
          >Choose a folder or type a new name. Leave empty for My
          uploads.</small
        >
        <p class="upload-access">
          Visible to people with access to this folder and your administrator.
        </p>
      </div>
      <div v-if="options" class="upload-storage">
        <span
          >{{
            bytes(Math.max(0, options.quota - options.used - options.reserved))
          }}
          available</span
        >
        <small>of {{ bytes(options.quota) }}</small>
        <progress
          :value="Math.min(options.quota, options.used + options.reserved)"
          :max="options.quota || 1"
          aria-label="Storage used and reserved"
        />
      </div>
      <section
        v-if="pending.length"
        class="pending-uploads"
        aria-label="Paused uploads"
      >
        <h3>Continue an upload</h3>
        <p>Choose the same file to pick up where you left off.</p>
        <div v-for="session in pending" :key="session.id">
          <strong>{{ session.filename }}</strong
          ><small
            >{{ Math.round((session.received / session.size) * 100) }}% saved ·
            {{
              session.folder.replace(/^user-\d+\/?/, "") || "My uploads"
            }}</small
          >
          <button
            v-if="session.received === session.size"
            class="button"
            :disabled="busy"
            @click="finishPending(session)"
          >
            Finish saving
          </button>
          <button
            v-else
            class="button"
            :disabled="busy"
            @click="resume(session)"
          >
            Choose original to resume
          </button>
          <button
            :disabled="busy || session.state === 'committing'"
            class="text-button"
            @click="discard(session)"
          >
            Discard transfer
          </button>
        </div>
      </section>
      <p v-if="optionsError" class="error" role="alert">
        {{ optionsError }} <button class="button" @click="open">Retry</button>
      </p>
      <p v-if="selectionNotice" role="status">{{ selectionNotice }}</p>
      <details class="upload-options">
        <summary>More options & file types</summary>
        <div class="upload-options-body">
          <label for="upload-duplicates">If a file is already uploaded</label>
          <select id="upload-duplicates" v-model="keepCopies" :disabled="busy">
            <option :value="false">Skip it (recommended)</option>
            <option :value="true">Upload another copy</option>
          </select>
          <p v-if="options">
            Interrupted uploads can be resumed for
            {{
              options.retentionHours >= 24
                ? `${Math.round(options.retentionHours / 24)} days`
                : `${options.retentionHours} hours`
            }}
            after their last progress. Reopen Upload and choose the same file.
          </p>
          <p v-if="options">
            Storage includes files in Trash and recovered files.
            {{ bytes(options.reserved) }} is reserved for unfinished uploads.
          </p>
          <details class="upload-formats">
            <summary>Supported file types</summary>
            <p v-for="group in formatGroups" :key="group.name">
              <strong>{{ group.name }}</strong
              ><br />{{ group.extensions }}
            </p>
            <p>
              Your original files are kept unchanged. Preview and video playback
              support can vary by format and browser.
            </p>
          </details>
        </div>
      </details>
      <p v-if="items.length" class="upload-selection-count">
        {{ items.length }} file{{ items.length === 1 ? "" : "s" }} selected
      </p>
      <ul v-if="items.length" class="upload-list">
        <li v-for="(item, index) in items" :key="index">
          <strong>{{ item.file.name }}</strong
          ><span role="status">{{ item.status }}</span
          ><progress
            v-if="
              ['Uploading', 'Checking file', 'Reconnecting'].includes(
                item.status,
              )
            "
            :value="item.progress"
            max="100"
            :aria-label="`Uploading ${item.file.name}`"
          /><small v-if="item.error" role="alert">{{ item.error }}</small>
        </li>
      </ul>
      <p v-if="items.some((i) => i.status === 'Saved')" role="status">
        Files saved. Previews and metadata are being prepared.
      </p>
      <footer>
        <button v-if="!busy" class="upload-cancel" @click="dialog?.close()">
          Cancel
        </button>
        <button v-if="busy" class="button" @click="stop">Pause uploads</button
        ><button
          class="primary"
          :disabled="
            busy ||
            optionsLoading ||
            !!optionsError ||
            !options?.enabled ||
            !items.length ||
            items.every((i) => ['Saved', 'Already uploaded'].includes(i.status))
          "
          @click="upload"
        >
          {{
            busy
              ? "Working…"
              : items.some((i) => !!i.session)
                ? "Resume uploads"
                : items.length
                  ? `Upload ${items.length} file${items.length === 1 ? "" : "s"}`
                  : "Upload"
          }}
        </button>
      </footer>
    </dialog></Teleport
  >
</template>

<style scoped>
.upload-dialog {
  width: min(560px, calc(100vw - 32px));
  padding: 28px;
}
.upload-dialog header {
  margin-bottom: 18px;
}
.upload-dialog h2 {
  font-size: 23px;
  letter-spacing: -0.03em;
}
.upload-dialog .upload-drop {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  gap: 12px;
  min-height: 130px;
  border-radius: var(--radius-panel);
  border: 1px dashed var(--muted);
  background: var(--bg);
  color: var(--text);
}
.upload-dialog .upload-drop:hover,
.upload-dialog .upload-drop.dragging {
  background: var(--active);
  border-color: var(--accent);
}
.upload-drop svg {
  color: var(--accent);
}
.upload-drop strong {
  font-size: 16px;
}
.upload-drop span {
  font-size: 13px;
  color: var(--muted);
}
.upload-dialog .upload-help {
  font-size: 12px;
  text-align: center;
  margin: 10px 0 18px;
}
.upload-destination {
  display: grid;
  gap: 8px;
}
.upload-destination label,
.upload-options label {
  font-size: 14px;
  font-weight: 600;
}
.upload-destination input,
.upload-options select {
  width: 100%;
  min-width: 0;
  min-height: 44px;
  padding: 11px 13px;
  background: var(--bg);
  color: var(--text);
  border: 1px solid var(--line);
  border-radius: var(--radius-control);
}
.upload-destination input:focus-visible,
.upload-options select:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}
.upload-destination small,
.upload-dialog .upload-access {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.5;
  margin: 0;
}
.upload-storage {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 6px;
  margin: 18px 0;
  font-size: 13px;
}
.upload-storage small {
  color: var(--muted);
}
.upload-storage progress {
  width: 100%;
  height: 5px;
  accent-color: var(--accent);
  margin-top: 4px;
}
.upload-options {
  border-top: 1px solid var(--line);
  padding: 15px 0;
}
.upload-options summary {
  cursor: pointer;
  font-size: 13px;
  color: var(--muted);
}
.upload-options-body {
  padding-top: 16px;
}
.upload-options summary:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 4px;
  border-radius: 4px;
}
.upload-dialog footer {
  position: sticky;
  bottom: -28px;
  background: var(--surface);
  padding: 18px 0 0;
  margin-top: 10px;
  border-top: 1px solid var(--line);
  justify-content: space-between;
  gap: 12px;
}
.upload-cancel {
  padding: 11px 16px;
  border: 1px solid var(--line);
  border-radius: var(--radius-control);
  background: transparent;
  color: var(--text);
}
.upload-selection-count {
  font-weight: 600;
}
@media (max-width: 600px) {
  .upload-dialog {
    padding: 20px;
  }
  .upload-dialog footer {
    bottom: -20px;
  }
}

.pending-uploads {
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 14px;
  margin: 16px 0;
}
.pending-uploads div {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin: 12px 0;
}
.pending-uploads strong,
.pending-uploads small {
  width: 100%;
  overflow-wrap: anywhere;
}
select {
  display: block;
  padding: 10px;
  margin: 8px 0;
  width: 100%;
}
</style>
