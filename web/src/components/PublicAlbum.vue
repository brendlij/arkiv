<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, nextTick } from "vue";
import Logo from "./Logo.vue";
import Icon from "./Icon.vue";
type Item = {
  id: number;
  filename: string;
  mediaType: string;
  previewStatus: string;
};
type Shared = {
  name: string;
  description: string;
  items: Item[];
  nextCursor: string;
  allowDownloads: boolean;
  expiresAt: number;
};
const token = location.pathname.split("/")[2] || "",
  base = "/api/public/" + encodeURIComponent(token),
  data = ref<Shared>(),
  error = ref(""),
  loading = ref(false),
  selected = ref<Item>(),
  cursors = ref([""]),
  page = ref(0),
  now = ref(Date.now());
const dialog = ref<HTMLDialogElement>();
watch(selected, async (value) => {
  if (value) {
    await nextTick();
    if (dialog.value && !dialog.value.open) dialog.value.showModal();
  } else dialog.value?.close();
});
let timer: ReturnType<typeof setInterval> | undefined;
function media(a: Item, size = 1024) {
  return `${base}/assets/${a.id}/thumbnail/${size}`;
}
async function load() {
  loading.value = true;
  error.value = "";
  selected.value = undefined;
  try {
    const r = await fetch(
      base + "?" + new URLSearchParams({ cursor: cursors.value[page.value] }),
      { credentials: "omit", cache: "no-store", referrerPolicy: "no-referrer" },
    );
    if (!r.ok)
      throw new Error(
        r.status === 404
          ? "This link has expired, was revoked, or is not available yet."
          : "This album could not be loaded. Please try again.",
      );
    data.value = await r.json();
  } catch (e) {
    data.value = undefined;
    error.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}
function next() {
  if (!data.value?.nextCursor) return;
  cursors.value[page.value + 1] = data.value.nextCursor;
  page.value++;
  void load();
}
function previous() {
  page.value--;
  void load();
}
function key(e: KeyboardEvent) {
  if (e.key === "Escape") selected.value = undefined;
  if (!selected.value || !data.value) return;
  const i = data.value.items.findIndex((a) => a.id === selected.value!.id),
    n = e.key === "ArrowRight" ? i + 1 : e.key === "ArrowLeft" ? i - 1 : i;
  if (n >= 0 && n < data.value.items.length)
    selected.value = data.value.items[n];
}
onMounted(() => {
  void load();
  document.addEventListener("keydown", key);
  timer = setInterval(() => {
    now.value = Date.now();
    if (data.value?.expiresAt && data.value.expiresAt * 1000 <= now.value) {
      data.value = undefined;
      selected.value = undefined;
      error.value = "This sharing link has expired.";
    }
  }, 1000);
});
onUnmounted(() => {
  clearInterval(timer);
  document.removeEventListener("keydown", key);
});
</script>
<template>
  <main class="public-album">
    <header class="public-brand">
      <a href="/" class="brand"><Logo /><span>arkiv</span></a
      ><span class="permission-pill">Shared album</span>
    </header>
    <section v-if="error" class="empty-state" role="alert">
      <Icon name="albums" />
      <h1>Album unavailable</h1>
      <p>{{ error }}</p>
      <button class="button" @click="load">Try again</button>
    </section>
    <p v-else-if="loading" role="status">Opening your shared moments…</p>
    <template v-else-if="data"
      ><div class="page-heading">
        <div>
          <p class="eyebrow">MOMENTS, SHARED</p>
          <h1>{{ data.name }}</h1>
          <p>{{ data.description }}</p>
        </div>
      </div>
      <p v-if="data.expiresAt" class="public-expiry">
        Available until {{ new Date(data.expiresAt * 1000).toLocaleString() }}
      </p>
      <div class="photo-grid">
        <button
          v-for="item in data.items"
          :key="item.id"
          class="photo"
          @click="selected = item"
          :aria-label="'Open ' + item.filename"
        >
          <img
            v-if="item.previewStatus === 'ready'"
            :src="media(item, 256)"
            :alt="item.filename"
            loading="lazy"
          /><Icon v-else name="photos" /><span class="photo-caption">{{
            item.filename
          }}</span>
        </button>
      </div>
      <p v-if="!data.items.length">No photos are available in this album.</p>
      <footer class="pagination">
        <button class="button" :disabled="page === 0" @click="previous">
          Previous</button
        ><span>Page {{ page + 1 }}</span
        ><button class="button" :disabled="!data.nextCursor" @click="next">
          Next
        </button>
      </footer></template
    >
    <dialog
      ref="dialog"
      v-if="selected && data"
      class="public-viewer"
      @cancel="selected = undefined"
      aria-modal="true"
      :aria-label="selected.filename"
      @click.self="selected = undefined"
    >
      <button
        class="public-close icon-button"
        aria-label="Close photo"
        @click="selected = undefined"
      >
        <Icon name="close" /></button
      ><video
        v-if="selected.mediaType === 'video' && data.allowDownloads"
        :key="selected.id"
        :src="`${base}/assets/${selected.id}/original`"
        :poster="media(selected)"
        controls
        autoplay
        playsinline
      /><img v-else :src="media(selected, 2048)" :alt="selected.filename" />
      <div class="public-caption">
        <span>{{ selected.filename }}</span>
        <p v-if="selected.mediaType === 'video' && !data.allowDownloads">
          Video preview only. Original playback is disabled by the owner.
        </p>
        <a
          v-if="data.allowDownloads"
          class="button"
          :href="`${base}/assets/${selected.id}/original?download=1`"
          download
          >Download original</a
        >
      </div>
    </dialog>
    <footer class="page-footer">A little collection, shared with you.</footer>
  </main>
</template>
