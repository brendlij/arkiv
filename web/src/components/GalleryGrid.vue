<script setup lang="ts">
import { computed, ref, watch, onUnmounted } from "vue";
import type { Asset } from "../types";
import { api, thumbnail } from "../api";
import Icon from "./Icon.vue";
const props = defineProps<{
  assets: Asset[];
  loading: boolean;
  trash?: boolean;
}>();
const emit = defineEmits<{ open: [index: number]; changed: [] }>();
const selected = ref(new Set<number>());
const selecting = ref(false),
  busy = ref(false),
  error = ref("");
const moveDialog = ref<HTMLDialogElement>();
const deleteDialog = ref<HTMLDialogElement>();
const ownedCount = computed(
  () => chosen.value.filter((a) => a.canManage).length,
);
function confirmDelete() {
  if (!busy.value && chosen.value.length) {
    error.value = "";
    deleteDialog.value?.showModal();
  }
}
const resultMessage = ref("");
const destination = ref("");
const destinations = ref<string[]>([]);
async function showMove() {
  destination.value = "";
  error.value = "";
  moveDialog.value?.showModal();
  try {
    destinations.value = await api<string[]>("/me/upload-folders");
  } catch (e) {
    error.value = (e as Error).message;
  }
}
let anchor = -1;
const chosen = computed(() =>
  props.assets.filter((a) => selected.value.has(a.id)),
);
const manageable = computed(
  () => chosen.value.length > 0 && chosen.value.every((a) => a.canManage),
);
function clear() {
  cancelHold();
  selected.value = new Set();
  selecting.value = false;
  anchor = -1;
}
watch(
  () => props.loading,
  (value) => {
    if (value) {
      cancelHold();
      clear();
    }
  },
);
function select(index: number, event: MouseEvent) {
  if (busy.value) return;
  const id = props.assets[index]!.id;
  if (event.shiftKey && anchor >= 0) {
    for (let i = Math.min(anchor, index); i <= Math.max(anchor, index); i++)
      selected.value.add(props.assets[i]!.id);
  } else {
    if (selected.value.has(id)) selected.value.delete(id);
    else selected.value.add(id);
    anchor = index;
  }
}
const gridRoot = ref<HTMLElement>();
let holdTimer: ReturnType<typeof setTimeout> | undefined;
let dragFrame = 0,
  dragActive = false,
  touchPending = false;
let holdX = 0,
  holdY = 0,
  dragX = 0,
  dragY = 0,
  dragStartIndex = -1,
  dragLastIndex = -1;
let suppressClickUntil = 0;
let dragBaseline = new Set<number>();
function cancelHold() {
  clearTimeout(holdTimer);
  cancelAnimationFrame(dragFrame);
  dragActive = touchPending = false;
}
function selectDragRange(index: number) {
  if (index === dragLastIndex) return;
  dragLastIndex = index;
  const next = new Set(dragBaseline);
  for (
    let i = Math.min(dragStartIndex, index);
    i <= Math.max(dragStartIndex, index);
    i++
  ) {
    const item = props.assets[i];
    if (item) next.add(item.id);
  }
  selected.value = next;
}
function selectAtFinger() {
  const target = document
    .elementFromPoint(dragX, dragY)
    ?.closest<HTMLElement>("[data-photo-index]");
  if (!target || !gridRoot.value?.contains(target)) return;
  const index = Number(target.dataset.photoIndex);
  if (Number.isInteger(index) && index >= 0 && index < props.assets.length)
    selectDragRange(index);
}
function scrollSelection() {
  if (!dragActive) return;
  const barBottom =
    gridRoot.value?.querySelector(".selection-toolbar")?.getBoundingClientRect()
      .bottom || 0;
  const top = Math.max(60, Math.min(barBottom + 24, window.innerHeight / 3));
  const bottom = window.innerHeight - 100;
  const speed =
    dragY < top
      ? -Math.min(12, (top - dragY) / 5)
      : dragY > bottom
        ? Math.min(12, (dragY - bottom) / 5)
        : 0;
  if (speed) {
    window.scrollBy(0, speed);
    selectAtFinger();
  }
  dragFrame = requestAnimationFrame(scrollSelection);
}
function hold(index: number, event: TouchEvent) {
  cancelHold();
  suppressClickUntil = 0;
  if (event.touches.length !== 1 || busy.value) return;
  touchPending = true;
  holdX = dragX = event.touches[0].clientX;
  holdY = dragY = event.touches[0].clientY;
  const id = props.assets[index]?.id;
  holdTimer = setTimeout(() => {
    if (!touchPending || props.assets[index]?.id !== id) return;
    dragBaseline = new Set(selected.value);
    dragStartIndex = index;
    dragLastIndex = -1;
    dragActive = true;
    selecting.value = true;
    anchor = index;
    selectDragRange(index);
    dragFrame = requestAnimationFrame(scrollSelection);
  }, 400);
}
function holdMove(event: TouchEvent) {
  if (event.touches.length !== 1) {
    endHold();
    return;
  }
  dragX = event.touches[0].clientX;
  dragY = event.touches[0].clientY;
  if (dragActive) {
    event.preventDefault();
    selectAtFinger();
  } else if (Math.hypot(dragX - holdX, dragY - holdY) > 10) cancelHold();
}
function endHold(event?: TouchEvent) {
  if (dragActive) {
    event?.preventDefault();
    suppressClickUntil = Date.now() + 800;
  }
  cancelHold();
}
onUnmounted(cancelHold);
function open(index: number, event: MouseEvent) {
  if (Date.now() < suppressClickUntil) return;
  if (
    selecting.value ||
    selected.value.size ||
    event.ctrlKey ||
    event.metaKey ||
    event.shiftKey
  )
    select(index, event);
  else {
    anchor = index;
    emit("open", index);
  }
}
function selectAll() {
  selected.value = new Set(props.assets.map((a) => a.id));
  selecting.value = true;
}
function keys(event: KeyboardEvent) {
  if ((event.target as HTMLElement).closest("input,textarea,dialog")) return;
  if (event.key === "Escape") {
    clear();
    event.preventDefault();
  }
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "a") {
    selectAll();
    event.preventDefault();
  }
  if (event.key === "Delete" && chosen.value.length && !busy.value) {
    if (props.trash) confirmDelete();
    else void perform("trash");
    event.preventDefault();
  }
}
async function perform(action: "move" | "trash" | "restore" | "delete") {
  if (
    busy.value ||
    !chosen.value.length ||
    (action === "move" && !manageable.value)
  )
    return;
  if (selected.value.size > 240) {
    error.value = "Select up to 240 items per action.";
    return;
  }
  busy.value = true;
  error.value = "";
  try {
    await api("/assets/bulk", "POST", {
      ids: [...selected.value],
      action,
      confirm: action === "delete",
      folder: destination.value,
    });
    resultMessage.value = `${selected.value.size} items ${action === "trash" ? "moved to Trash" : action === "restore" ? "restored" : action === "delete" ? "removed permanently from Trash. Original-file cleanup may finish in the background" : "moved"}.`;
    moveDialog.value?.close();
    deleteDialog.value?.close();
    clear();
    emit("changed");
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
const failed = ref(new Set<number>());
watch(
  () => props.assets,
  () => {
    failed.value = new Set();
  },
);
const groups = computed(() => {
  const result: {
    date: string;
    label: string;
    items: { asset: Asset; index: number }[];
  }[] = [];
  props.assets.forEach((asset, index) => {
    const date = new Date(asset.takenAt);
    const key = date.toLocaleDateString();
    let group = result[result.length - 1];
    if (!group || group.date !== key) {
      group = {
        date: key,
        label: date.toLocaleDateString(undefined, {
          weekday: "long",
          day: "numeric",
          month: "long",
          year: "numeric",
        }),
        items: [],
      };
      result.push(group);
    }
    group.items.push({ asset, index });
  });
  return result;
});
function duration(seconds: number) {
  return `${Math.floor(seconds / 60)}:${Math.floor(seconds % 60)
    .toString()
    .padStart(2, "0")}`;
}
</script>
<template>
  <div ref="gridRoot" class="selectable-gallery" @keydown="keys">
    <div
      v-if="!loading && assets.length"
      class="selection-toolbar"
      :class="{ 'selection-idle': !selected.size && !selecting }"
      aria-label="Media selection"
    >
      <template v-if="selected.size || selecting">
        <strong aria-live="polite">{{ selected.size }} selected</strong>
        <button :disabled="busy" @click="selectAll">Select loaded items</button>
        <button v-if="!trash" :disabled="!manageable || busy" @click="showMove">
          Move…
        </button>
        <button
          v-if="!trash"
          :disabled="!chosen.length || busy"
          @click="perform('trash')"
        >
          Move to Trash
        </button>
        <button
          v-else
          :disabled="!chosen.length || busy"
          @click="perform('restore')"
        >
          Restore
        </button>
        <button
          v-if="trash"
          :disabled="!chosen.length || busy"
          @click="confirmDelete"
        >
          {{ ownedCount ? "Delete permanently…" : "Remove permanently…" }}
        </button>
        <button :disabled="busy" @click="clear">Clear selection</button>
        <small v-if="chosen.length && !manageable"
          >Only your own uploads can be moved between folders. Other photos are
          removed from your view; their originals stay with their owner or
          server folder.</small
        >
      </template>
      <template v-else
        ><button @click="selecting = true">Select items</button
        ><small
          >Ctrl / Strg-click to select · Shift-click for a range</small
        ></template
      >
    </div>
    <div
      v-if="!loading && assets.length"
      class="mobile-selection-strip"
      aria-label="Photo selection"
    >
      <span v-if="selecting || selected.size" role="status"
        >{{ selected.size }} selected</span
      >
      <span v-else class="selection-hint">Hold a photo to select</span>
      <div>
        <button
          v-if="selecting || selected.size"
          :disabled="busy"
          @click="selectAll"
        >
          Select all
        </button>
        <button
          :disabled="busy"
          @click="selecting || selected.size ? clear() : (selecting = true)"
        >
          {{ selecting || selected.size ? "Done" : "Select" }}
        </button>
      </div>
    </div>
    <div
      v-if="!loading && (selecting || selected.size)"
      class="mobile-selection-dock"
      role="group"
      aria-label="Selected photo actions"
    >
      <button v-if="!trash" :disabled="!manageable || busy" @click="showMove">
        <Icon name="folder" /><span>Move</span>
      </button>
      <button
        v-if="!trash"
        :disabled="!chosen.length || busy"
        @click="perform('trash')"
      >
        <Icon name="inbox" /><span>Trash</span>
      </button>
      <button
        v-if="trash"
        :disabled="!chosen.length || busy"
        @click="perform('restore')"
      >
        <Icon name="refresh" /><span>Restore</span>
      </button>
      <button
        v-if="trash"
        :disabled="!chosen.length || busy"
        @click="confirmDelete"
      >
        <Icon name="close" /><span>Delete</span>
      </button>
      <button :disabled="busy" @click="clear" aria-label="Finish selection">
        <Icon name="close" /><span>Done ({{ selected.size }})</span>
      </button>
    </div>
    <p v-if="resultMessage" role="status">{{ resultMessage }}</p>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="trash">
      Restore items or remove them permanently. Your uploaded originals still
      count toward storage until deleted. Shared and server-library photos are
      removed only from your view.
    </p>
    <dialog
      ref="deleteDialog"
      class="move-dialog"
      aria-labelledby="delete-title"
      @cancel="busy && $event.preventDefault()"
    >
      <form @submit.prevent="perform('delete')">
        <h2 id="delete-title">
          {{ ownedCount ? "Delete permanently?" : "Remove permanently?" }}
        </h2>
        <p v-if="ownedCount">
          {{ ownedCount }} uploaded original{{ ownedCount === 1 ? "" : "s" }}
          will be deleted from storage, including their album and share entries.
          This cannot be undone.
        </p>
        <p v-if="chosen.length > ownedCount">
          {{ chosen.length - ownedCount }} shared or server-library item(s) will
          be permanently hidden from your view. Their original files and other
          people's access stay unchanged.
        </p>
        <ul class="delete-list">
          <li v-for="item in chosen" :key="item.id">
            {{ item.filename }} —
            {{ item.canManage ? "Delete original" : "Remove from my view" }}
          </li>
        </ul>
        <p v-if="error" role="alert">{{ error }}</p>
        <footer>
          <button type="button" :disabled="busy" @click="deleteDialog?.close()">
            Cancel</button
          ><button :disabled="busy">
            {{
              busy
                ? "Removing…"
                : ownedCount
                  ? "Delete permanently"
                  : "Remove permanently"
            }}
          </button>
        </footer>
      </form>
    </dialog>
    <dialog ref="moveDialog" class="move-dialog" aria-labelledby="move-title">
      <form @submit.prevent="perform('move')">
        <h2 id="move-title">Move {{ selected.size }} items</h2>
        <label for="destination">Folder inside My uploads</label>
        <input
          list="upload-destinations"
          id="destination"
          v-model="destination"
          placeholder="e.g. Holidays/Summer"
          autofocus
          :disabled="busy"
        />
        <datalist id="upload-destinations">
          <option
            v-for="folder in destinations"
            :key="folder"
            :value="folder"
          />
        </datalist>
        <p>
          Use / for subfolders, or leave empty for My uploads. New folders are
          created automatically.
        </p>
        <p v-if="error" role="alert">{{ error }}</p>
        <footer>
          <button type="button" :disabled="busy" @click="moveDialog?.close()">
            Cancel</button
          ><button :disabled="busy">
            {{ busy ? "Moving…" : "Move here" }}
          </button>
        </footer>
      </form>
    </dialog>
    <div
      v-if="loading"
      class="skeleton-grid"
      aria-label="Loading photos"
      aria-busy="true"
    >
      <div v-for="i in 12" :key="i" />
    </div>
    <section v-for="group in groups" v-else :key="group.date" class="day-group">
      <header class="day-heading">
        <h2>{{ group.label }}</h2>
        <span>{{ group.items.length }} memories</span>
      </header>
      <div class="photo-grid">
        <div
          v-for="{ asset, index } in group.items"
          :key="asset.id"
          class="photo selection-photo"
          :data-photo-index="index"
          :class="{
            portrait: asset.height > asset.width,
            selected: selected.has(asset.id),
          }"
          :style="{
            aspectRatio: `${asset.width || 4} / ${asset.height || 3}`,
          }"
        >
          <button
            class="photo-open"
            :aria-label="`${selecting || selected.size ? 'Select' : 'Open'} ${asset.filename}`"
            :aria-pressed="
              selecting || selected.size ? selected.has(asset.id) : undefined
            "
            @click="open(index, $event)"
            @touchstart="hold(index, $event)"
            @touchmove="holdMove"
            @touchend="endHold"
            @touchcancel="endHold"
            @contextmenu.prevent
          ></button>
          <button
            class="photo-select"
            :class="{ visible: selecting || selected.size }"
            :aria-label="`Select ${asset.filename}`"
            :aria-pressed="selected.has(asset.id)"
            :disabled="busy"
            @click.stop="select(index, $event)"
          >
            {{ selected.has(asset.id) ? "✓" : "" }}
          </button>
          <img
            v-if="asset.previewStatus === 'ready' && !failed.has(asset.id)"
            :src="thumbnail(asset.id)"
            :srcset="`${thumbnail(asset.id)} 256w, ${thumbnail(asset.id, 1024)} 1024w`"
            sizes="(max-width:700px) 32vw, (max-width:1000px) 45vw, 30vw"
            loading="lazy"
            decoding="async"
            :alt="asset.filename"
            @error="failed.add(asset.id)"
          />
          <div v-else class="image-fallback">
            <Icon
              :name="asset.mediaType === 'video' ? 'play' : 'photos'"
            /><span>{{
              asset.previewStatus === "pending"
                ? "Preparing preview"
                : "Preview unavailable"
            }}</span
            ><small>{{ asset.filename }}</small>
          </div>
          <span v-if="asset.favorite" class="photo-favorite"
            ><Icon name="heart" /></span
          ><span v-if="asset.mediaType === 'raw'" class="media-badge">RAW</span
          ><span v-if="asset.mediaType === 'video'" class="media-badge"
            ><Icon name="play" />{{ duration(asset.duration) }}</span
          ><span class="photo-caption">{{ asset.filename }}</span>
        </div>
      </div>
    </section>
  </div>
</template>
<style scoped>
.selection-toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  margin: 0 0 22px;
  padding: 12px;
  border: 0;
  box-shadow: 0 0 0 8px var(--bg);
  isolation: isolate;
  border-radius: 12px;
  background: var(--surface, #faf9f5);
  position: sticky;
  top: 0;
  z-index: 5;
}
.selection-toolbar button,
.move-dialog button {
  padding: 9px 14px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--surface);
  color: var(--accent);
  cursor: pointer;
}
.selection-toolbar:not(.selection-idle) button,
.move-dialog button {
  border: 0;
  background: var(--active);
}
.selection-toolbar:not(.selection-idle) button:hover:not(:disabled),
.move-dialog button:hover:not(:disabled) {
  background: var(--accent);
  color: var(--bg);
}
.selection-toolbar button:disabled {
  opacity: 1;
  background: var(--hover);
  color: var(--muted);
  cursor: default;
}
.selection-toolbar small {
  color: var(--muted);
}
.selection-toolbar.selection-idle {
  position: static;
  padding: 0;
  margin-bottom: 16px;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  gap: 12px;
}
.selection-idle button {
  padding: 5px 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  font-size: 13px;
}
.selection-idle button:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}
.selection-idle small {
  font-size: 11px;
}
@media (max-width: 600px) {
  .selection-idle small {
    display: none;
  }
}
.selection-photo {
  position: relative;
}
.selection-photo.selected::after {
  content: "";
  position: absolute;
  inset: 0;
  z-index: 4;
  border: 4px solid var(--accent);
  border-radius: inherit;
  pointer-events: none;
}
.photo-open {
  position: absolute;
  inset: 0;
  z-index: 2;
  border: 0;
  background: transparent;
  cursor: pointer;
}
.photo-select {
  position: absolute;
  top: 10px;
  left: 10px;
  width: 28px;
  height: 28px;
  z-index: 3;
  border: 2px solid white;
  border-radius: 8px;
  color: white;
  background: #164c45;
  opacity: 0;
  cursor: pointer;
}
.selection-photo:hover .photo-select,
.photo-select:focus-visible,
.photo-select.visible {
  opacity: 1;
}
.move-dialog {
  max-width: 460px;
  width: calc(100% - 40px);
  padding: 28px;
  border: 1px solid var(--line);
  border-radius: 16px;
  background: var(--surface);
  color: var(--text);
}
.delete-list {
  max-height: 220px;
  overflow: auto;
  overflow-wrap: anywhere;
  padding-left: 20px;
}
.move-dialog::backdrop {
  background: #142d2855;
}
.move-dialog input {
  width: 100%;
  margin: 12px 0;
  padding: 12px;
}
.move-dialog footer {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  margin-top: 24px;
}
@media (hover: none), (max-width: 700px) {
  .photo-select {
    display: none;
  }
  .photo-open,
  .selection-photo img {
    -webkit-touch-callout: none;
    user-select: none;
    -webkit-user-select: none;
  }
}
</style>

<style scoped>
@media (max-width: 700px) {
  .selection-toolbar {
    gap: 6px;
    padding: 8px;
    font-size: 12px;
  }
  .selection-toolbar button {
    min-height: 44px;
    padding: 8px 10px;
  }
  .selection-toolbar small {
    flex-basis: 100%;
  }
  .selection-photo {
    content-visibility: auto;
    contain-intrinsic-size: auto 120px;
  }
}
</style>

<style scoped>
.mobile-selection-strip,
.mobile-selection-dock {
  display: none;
}
@media (max-width: 700px) {
  .selection-toolbar {
    display: none;
  }
  .mobile-selection-strip {
    display: flex;
    height: 44px;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-bottom: 14px;
    font-size: 13px;
    color: var(--accent);
  }
  .mobile-selection-strip > span {
    white-space: nowrap;
  }
  .selection-hint {
    color: var(--muted);
    font-size: 12px;
  }
  .mobile-selection-strip > div {
    display: flex;
    gap: 12px;
  }
  .mobile-selection-strip button {
    min-height: 44px;
    padding: 0 4px;
    color: var(--accent);
    font-weight: 500;
    background: transparent;
    border: 0;
  }
  .mobile-selection-dock {
    position: fixed;
    z-index: 40;
    inset: auto 0 0;
    height: calc(64px + env(safe-area-inset-bottom));
    padding: 4px 20px calc(4px + env(safe-area-inset-bottom));
    display: flex;
    align-items: stretch;
    justify-content: space-evenly;
    border-top: 1px solid var(--line);
    background: var(--panel);
  }
  .mobile-selection-dock button {
    display: flex;
    flex: 1;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 4px;
    border: 0;
    background: transparent;
    color: var(--accent);
    font-size: 12px;
    min-height: 44px;
  }
  .mobile-selection-dock button:disabled {
    opacity: 1;
    color: var(--muted);
  }
  .mobile-selection-dock svg {
    width: 20px;
    height: 20px;
  }
}
</style>
