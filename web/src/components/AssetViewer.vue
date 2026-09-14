<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch, nextTick } from "vue";
import type { Asset, Album } from "../types";
import { api, thumbnail } from "../api";
import AccessDetails from "./AccessDetails.vue";
import Icon from "./Icon.vue";
const props = defineProps<{
  assets: Asset[];
  index: number;
  albumId?: number;
  canEdit?: boolean;
}>();
const emit = defineEmits<{
  close: [];
  navigate: [index: number];
  favorite: [asset: Asset];
  removed: [id: number];
}>();
const asset = computed(() => props.assets[props.index]);
const info = ref(false),
  error = ref(""),
  albumMenu = ref(false),
  albums = ref<Album[]>([]),
  busy = ref(false),
  previewFailed = ref(false);
const albumOffset = ref(0),
  moreAlbums = ref(false);
const dialog = ref<HTMLElement>(),
  closeButton = ref<HTMLButtonElement>();
const stage = ref<HTMLElement>(),
  photoImage = ref<HTMLImageElement>();
const video = ref<HTMLVideoElement>();
const mobileViewer = ref(matchMedia("(max-width: 700px)").matches);
function updateMobileViewer() {
  mobileViewer.value = matchMedia("(max-width: 700px)").matches;
}
const videoReady = ref(false),
  playing = ref(false),
  videoTime = ref(0),
  videoDuration = ref(0);
const compatible = ref(false),
  playbackState = ref("");
let playbackTimer: ReturnType<typeof setTimeout> | undefined;
let playbackEpoch = 0;
function resetPlayback() {
  playbackEpoch++;
  clearTimeout(playbackTimer);
  compatible.value = false;
  playbackState.value = "";
}
async function preparePlayback() {
  const id = asset.value.id,
    epoch = ++playbackEpoch;
  clearTimeout(playbackTimer);
  playbackState.value = "pending";
  error.value = "";
  async function check(method: "GET" | "POST") {
    try {
      const result = await api<{ state: string; error: string }>(
        `/assets/${id}/playback`,
        method,
      );
      if (epoch !== playbackEpoch) return;
      playbackState.value = result.state;
      if (result.state === "ready") {
        video.value?.pause();
        videoReady.value = false;
        resetZoom();
        compatible.value = true;
      } else if (result.state === "pending" || result.state === "running")
        playbackTimer = setTimeout(() => void check("GET"), 2000);
      else
        error.value = result.error || "Playback copy unavailable. Try again.";
    } catch (e) {
      if (epoch === playbackEpoch) {
        playbackState.value = "failed";
        error.value = String(e);
      }
    }
  }
  await check("POST");
}
const originalReady = ref(false),
  originalLoading = ref(false),
  originalFailed = ref(false);
const fullImageSource = ref("");
const photoSource = computed(
  () => fullImageSource.value || `/api/assets/${asset.value.id}/original`,
);
function loadOriginal() {
  if (asset.value.mediaType === "video") return;
  originalLoading.value = true;
  originalFailed.value = false;
  originalReady.value = false;
  fullImageSource.value =
    asset.value.mediaType === "raw"
      ? `/api/assets/${asset.value.id}/full-image`
      : `/api/assets/${asset.value.id}/original`;
}
function imageError() {
  if (fullImageSource.value.endsWith("/original")) {
    fullImageSource.value = `/api/assets/${asset.value.id}/full-image`;
    return;
  }
  originalLoading.value = false;
  originalFailed.value = true;
  previewFailed.value = true;
}
function imageLoaded() {
  originalReady.value = true;
  originalLoading.value = false;
  clampPan();
}
async function togglePlayback() {
  const current = video.value;
  if (!current) return;
  try {
    if (current.paused) await current.play();
    else current.pause();
  } catch (e) {
    if (current === video.value) error.value = (e as Error).message;
  }
}
function seek(event: Event) {
  if (video.value)
    video.value.currentTime = Number((event.target as HTMLInputElement).value);
}
const scale = ref(1),
  panX = ref(0),
  panY = ref(0),
  dragging = ref(false);
const zoom = computed(() => scale.value > 1);
const canZoom = computed(() =>
  asset.value.mediaType === "video"
    ? videoReady.value
    : originalReady.value && !previewFailed.value,
);
let dragStart = { x: 0, y: 0, panX: 0, panY: 0 };
function resetZoom() {
  scale.value = 1;
  panX.value = panY.value = 0;
  dragging.value = false;
}
function clampPan() {
  const media =
    asset.value.mediaType === "video" ? video.value : photoImage.value;
  if (!stage.value || !media) return;
  const x = Math.max(
    0,
    (media.offsetWidth * scale.value - stage.value.clientWidth) / 2,
  );
  const y = Math.max(
    0,
    (media.offsetHeight * scale.value - stage.value.clientHeight) / 2,
  );
  panX.value = Math.max(-x, Math.min(x, panX.value));
  panY.value = Math.max(-y, Math.min(y, panY.value));
}
function changeZoom(value: number, clientX?: number, clientY?: number) {
  if (!canZoom.value || !stage.value) return;
  revealControls();
  const next = Math.max(1, Math.min(8, value));
  const bounds = stage.value.getBoundingClientRect();
  const x =
    (clientX ?? bounds.left + bounds.width / 2) -
    bounds.left -
    bounds.width / 2;
  const y =
    (clientY ?? bounds.top + bounds.height / 2) -
    bounds.top -
    bounds.height / 2;
  const ratio = next / scale.value;
  panX.value = x - (x - panX.value) * ratio;
  panY.value = y - (y - panY.value) * ratio;
  scale.value = next;
  clampPan();
}
function wheelZoom(event: WheelEvent) {
  if (!canZoom.value) return;
  event.preventDefault();
  const delta =
    event.deltaY *
    (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? 400 : 1);
  changeZoom(
    scale.value * Math.exp(-Math.max(-120, Math.min(120, delta)) * 0.003),
    event.clientX,
    event.clientY,
  );
}
function startPan(event: PointerEvent) {
  if (event.pointerType === "touch") return;
  if (
    !zoom.value ||
    event.button !== 0 ||
    (event.target as HTMLElement).closest("button,input,.video-zoom-controls")
  )
    return;
  event.preventDefault();
  dragging.value = true;
  dragStart = {
    x: event.clientX,
    y: event.clientY,
    panX: panX.value,
    panY: panY.value,
  };
  stage.value?.setPointerCapture(event.pointerId);
}
function movePan(event: PointerEvent) {
  if (!dragging.value) return;
  panX.value = dragStart.panX + event.clientX - dragStart.x;
  panY.value = dragStart.panY + event.clientY - dragStart.y;
  clampPan();
}
function endPan(event: PointerEvent) {
  dragging.value = false;
  if (stage.value?.hasPointerCapture(event.pointerId))
    stage.value.releasePointerCapture(event.pointerId);
}
watch(info, resetZoom);
let previous: Element | null = null;
let touchStart = { x: 0, y: 0, panX: 0, panY: 0, scale: 1, distance: 0 };
let touched = false,
  pinched = false,
  lastTap = 0;
let lastTouchAt = 0;
const swipeX = ref(0),
  swipeY = ref(0);
let tapTimer: ReturnType<typeof setTimeout> | undefined;
function toggleControls() {
  clearTimeout(idleTimer);
  controlsVisible.value = !controlsVisible.value;
  if (controlsVisible.value) revealControls();
}
function cancelTouch() {
  touched = false;
  swipeX.value = swipeY.value = 0;
}

let savedScroll = 0,
  savedBody = "";
function touchBegin(event: TouchEvent) {
  if (
    (event.target as HTMLElement).closest("button,input,.video-zoom-controls")
  ) {
    touched = false;
    return;
  }
  const a = event.touches[0];
  // Keep the native video timeline and playback controls available to touch.
  if (
    (event.target as HTMLElement).tagName === "VIDEO" &&
    !zoom.value &&
    !mobileViewer.value &&
    event.touches.length === 1
  ) {
    const bounds = (event.target as HTMLElement).getBoundingClientRect();
    if (a.clientY >= bounds.bottom - 64) {
      touched = false;
      return;
    }
  }
  swipeX.value = swipeY.value = 0;
  touched = true;
  pinched = event.touches.length > 1;
  touchStart = {
    x: a.clientX,
    y: a.clientY,
    panX: panX.value,
    panY: panY.value,
    scale: scale.value,
    distance: pinched
      ? Math.hypot(
          event.touches[1].clientX - a.clientX,
          event.touches[1].clientY - a.clientY,
        )
      : 0,
  };
}
function touchMove(event: TouchEvent) {
  if (!touched) return;
  const a = event.touches[0];
  if (event.touches.length > 1 && touchStart.distance > 0) {
    event.preventDefault();
    pinched = true;
    swipeX.value = swipeY.value = 0;
    clearTimeout(tapTimer);
    const b = event.touches[1];
    changeZoom(
      (touchStart.scale *
        Math.hypot(b.clientX - a.clientX, b.clientY - a.clientY)) /
        touchStart.distance,
      (a.clientX + b.clientX) / 2,
      (a.clientY + b.clientY) / 2,
    );
  } else if (zoom.value) {
    event.preventDefault();
    panX.value = touchStart.panX + a.clientX - touchStart.x;
    panY.value = touchStart.panY + a.clientY - touchStart.y;
    clampPan();
  } else if (!pinched) {
    const dx = a.clientX - touchStart.x,
      dy = a.clientY - touchStart.y;
    if (Math.abs(dx) > 10 || Math.abs(dy) > 10) {
      event.preventDefault();
      clearTimeout(tapTimer);
      if (Math.abs(dx) > Math.abs(dy) * 1.2) {
        const atEdge =
          (dx > 0 && props.index === 0) ||
          (dx < 0 && props.index === props.assets.length - 1);
        swipeX.value = dx * (atEdge ? 0.2 : 0.75);
        swipeY.value = 0;
      } else if (dy > 0 && Math.abs(dy) > Math.abs(dx) * 1.2) {
        swipeY.value = dy * 0.8;
        swipeX.value = 0;
      }
    }
  }
}
function touchEnd(event: TouchEvent) {
  if (!touched) return;
  lastTouchAt = Date.now();
  if (event.touches.length) {
    touchStart.x = event.touches[0].clientX;
    touchStart.y = event.touches[0].clientY;
    touchStart.panX = panX.value;
    touchStart.panY = panY.value;
    return;
  }
  touched = false;
  swipeX.value = swipeY.value = 0;
  if (pinched) {
    lastTap = 0;
    return;
  }
  const a = event.changedTouches[0],
    dx = a.clientX - touchStart.x,
    dy = a.clientY - touchStart.y;
  if (!zoom.value && Math.abs(dx) > 65 && Math.abs(dx) > Math.abs(dy) * 1.5) {
    event.preventDefault();
    clearTimeout(tapTimer);
    lastTap = 0;
    navigate(dx < 0 ? 1 : -1);
  } else if (!zoom.value && dy > 95 && dy > Math.abs(dx) * 1.5) {
    event.preventDefault();
    clearTimeout(tapTimer);
    lastTap = 0;
    emit("close");
  } else if (Math.abs(dx) < 12 && Math.abs(dy) < 12) {
    event.preventDefault();
    const now = Date.now();
    clearTimeout(tapTimer);
    if (now - lastTap < 300) {
      event.preventDefault();
      changeZoom(zoom.value ? 1 : 2.5, a.clientX, a.clientY);
      lastTap = 0;
    } else {
      lastTap = now;
      tapTimer = setTimeout(toggleControls, 300);
    }
  }
}

const controlsVisible = ref(true);
let keyboardActive = false;
let idleTimer: ReturnType<typeof setTimeout> | undefined;
function revealControls() {
  controlsVisible.value = true;
  clearTimeout(idleTimer);
  idleTimer = setTimeout(() => {
    if (!info.value && !albumMenu.value && !keyboardActive)
      controlsVisible.value = false;
  }, 2500);
}
watch(
  () => props.index,
  () => {
    resetPlayback();
    cancelTouch();
    clearTimeout(tapTimer);
    video.value?.pause();
    videoReady.value = playing.value = false;
    videoTime.value = videoDuration.value = 0;
    originalReady.value = originalLoading.value = originalFailed.value = false;
    resetZoom();
    error.value = "";
    previewFailed.value = false;
    albumMenu.value = false;
    loadOriginal();
  },
);
async function favorite() {
  if (busy.value) return;
  busy.value = true;
  const selected = { ...asset.value };
  try {
    await api(`/assets/${selected.id}/favorite`, "POST", {
      favorite: !selected.favorite,
    });
    emit("favorite", { ...selected, favorite: !selected.favorite });
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function showAlbums() {
  try {
    albums.value = [];
    albumOffset.value = 0;
    await loadAlbums();
    albumMenu.value = !albumMenu.value;
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function loadAlbums() {
  try {
    const page = await api<{ items: Album[]; hasMore: boolean }>(
      `/albums?offset=${albumOffset.value}`,
    );
    albums.value = page.items.filter((a) => a.role !== "viewer");
    moreAlbums.value = page.hasMore;
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function addAlbum(id: number) {
  try {
    await api(`/albums/${id}/assets`, "POST", { assetId: asset.value.id });
    albumMenu.value = false;
    error.value = "Added to album";
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function remove() {
  try {
    await api(`/albums/${props.albumId}/assets/${asset.value.id}`, "DELETE");
    emit("removed", asset.value.id);
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function fullscreen() {
  try {
    if (document.fullscreenElement) await document.exitFullscreen();
    else await dialog.value?.requestFullscreen();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
function navigate(delta: number) {
  const n = props.index + delta;
  if (n >= 0 && n < props.assets.length) emit("navigate", n);
}
function key(event: KeyboardEvent) {
  keyboardActive = true;
  revealControls();
  if (event.key === "Escape") {
    emit("close");
    return;
  }
  if (
    ["INPUT", "SELECT", "TEXTAREA"].includes(
      (event.target as HTMLElement)?.tagName,
    )
  )
    return;
  if ((event.target as HTMLElement)?.isContentEditable) return;
  if (
    event.code === "Space" &&
    asset.value.mediaType === "video" &&
    !event.ctrlKey &&
    !event.metaKey &&
    !event.altKey &&
    !albumMenu.value
  ) {
    event.preventDefault();
    event.stopPropagation();
    if (!event.repeat) void togglePlayback();
    return;
  }
  if (event.key === "ArrowRight") {
    event.preventDefault();
    navigate(1);
  }
  if (
    !event.ctrlKey &&
    !event.metaKey &&
    ["+", "=", "-", "0"].includes(event.key)
  ) {
    event.preventDefault();
    if (event.key === "0") resetZoom();
    else changeZoom(scale.value * (event.key === "-" ? 1 / 1.5 : 1.5));
  }
  if (event.key === "ArrowLeft") {
    event.preventDefault();
    navigate(-1);
  }
  if (event.key === "Tab") {
    const elements = dialog.value?.querySelectorAll<HTMLElement>(
      "button:not(:disabled),a[href],select,video",
    );
    if (!elements?.length) return;
    const first = elements[0],
      last = elements[elements.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }
}
onMounted(async () => {
  loadOriginal();
  revealControls();
  previous = document.activeElement;
  savedScroll = window.scrollY;
  savedBody = document.body.style.cssText;
  Object.assign(document.body.style, {
    overflow: "hidden",
    position: "fixed",
    top: `-${savedScroll}px`,
    width: "100%",
  });
  window.addEventListener("keydown", key, true);
  window.addEventListener("resize", resetZoom);
  window.addEventListener("resize", updateMobileViewer);
  document.addEventListener("fullscreenchange", resetZoom);
  await nextTick();
  closeButton.value?.focus();
});
onUnmounted(() => {
  clearTimeout(tapTimer);
  resetPlayback();
  clearTimeout(idleTimer);
  window.removeEventListener("keydown", key, true);
  video.value?.pause();
  window.removeEventListener("resize", resetZoom);
  window.removeEventListener("resize", updateMobileViewer);
  document.removeEventListener("fullscreenchange", resetZoom);
  document.body.style.cssText = savedBody;
  window.scrollTo(0, savedScroll);
  (previous as HTMLElement)?.focus({ preventScroll: true });
});
</script>
<template>
  <Teleport to="body"
    ><div
      ref="dialog"
      class="viewer"
      :class="{ 'controls-hidden': !controlsVisible }"
      @pointermove="
        if ($event.pointerType === 'mouse') {
          keyboardActive = false;
          revealControls();
        }
      "
      @pointerdown="
        if ($event.pointerType === 'mouse') {
          keyboardActive = false;
          revealControls();
        }
      "
      @focusin="revealControls"
      role="dialog"
      aria-modal="true"
      :aria-label="asset.filename"
    >
      <header class="viewer-toolbar">
        <button
          ref="closeButton"
          class="icon-button"
          aria-label="Close viewer"
          @click="emit('close')"
        >
          <Icon name="close" />
        </button>
        <div class="viewer-title">
          {{ asset.filename
          }}<small>{{ index + 1 }} / {{ assets.length }}</small>
        </div>
        <div class="viewer-actions">
          <button
            v-if="asset.mediaType === 'video'"
            class="icon-button"
            :aria-label="playing ? 'Pause video' : 'Play video'"
            title="Play / pause (Space)"
            @click="togglePlayback"
          >
            <Icon :name="playing ? 'pause' : 'play'" />
          </button>
          <button
            class="icon-button"
            :class="{ selected: asset.favorite }"
            :aria-pressed="asset.favorite"
            aria-label="Toggle favorite"
            :disabled="busy"
            @click="favorite"
          >
            <Icon name="heart" /></button
          ><button
            class="icon-button"
            aria-label="Add to album"
            title="Add to album"
            v-if="asset.canAdd"
            @click="showAlbums"
          >
            <Icon name="albumAdd" /></button
          ><button
            class="icon-button"
            aria-label="Zoom out"
            title="Zoom out (−)"
            :disabled="!canZoom || scale <= 1"
            @click="changeZoom(scale / 1.5)"
          >
            <Icon name="minus" />
          </button>
          <button
            class="zoom-level"
            aria-label="Fit to screen"
            title="Fit to screen (0). Percentage is relative to fitted size."
            :disabled="!canZoom"
            @click="resetZoom"
          >
            {{ scale === 1 ? "Fit" : `${Math.round(scale * 100)}%` }}
          </button>
          <button
            class="icon-button"
            aria-label="Zoom in"
            title="Zoom in (+)"
            :disabled="!canZoom || scale >= 8"
            @click="changeZoom(scale * 1.5)"
          >
            <Icon name="plus" /></button
          ><button
            class="icon-button fullscreen"
            aria-label="Fullscreen"
            @click="fullscreen"
          >
            <Icon name="expand" /></button
          ><button
            class="icon-button"
            aria-label="Photo details"
            :aria-pressed="info"
            @click="info = !info"
          >
            <Icon name="info" />
          </button>
        </div>
      </header>
      <div v-if="albumMenu" class="album-menu">
        <h3>Add to album</h3>
        <button
          v-for="album in albums"
          :key="album.id"
          @click="addAlbum(album.id)"
        >
          {{ album.name }}
        </button>
        <p v-if="!albums.length">Create an album from the Albums view first.</p>
        <button
          v-if="albumOffset"
          @click="
            albumOffset -= 200;
            loadAlbums();
          "
        >
          Previous albums
        </button>
        <button
          v-if="moreAlbums"
          @click="
            albumOffset += 200;
            loadAlbums();
          "
        >
          More albums
        </button>
        <button @click="albumMenu = false">Cancel</button>
      </div>
      <div class="viewer-body">
        <div
          class="viewer-stage"
          ref="stage"
          :class="{ zoomed: zoom, dragging }"
          @wheel="wheelZoom"
          @pointerdown="startPan"
          @pointermove="movePan"
          @pointerup="endPan"
          @pointercancel="endPan"
          @lostpointercapture="dragging = false"
          @touchstart="touchBegin"
          @touchmove="touchMove"
          @touchend="touchEnd"
          @touchcancel="cancelTouch"
        >
          <video
            ref="video"
            v-if="asset.mediaType === 'video'"
            :key="asset.id"
            :style="{
              transform: `translate(${panX + swipeX}px, ${panY + swipeY}px) scale(${scale})`,
            }"
            @loadedmetadata="
              videoReady = true;
              videoDuration = Number.isFinite(video?.duration)
                ? video!.duration
                : 0;
              clampPan();
            "
            @play="playing = true"
            @pause="playing = false"
            @ended="playing = false"
            @timeupdate="videoTime = video?.currentTime || 0"
            :src="
              compatible
                ? `/api/assets/${asset.id}/playback/file`
                : `/api/assets/${asset.id}/original`
            "
            :poster="
              asset.previewStatus === 'ready'
                ? thumbnail(asset.id, 1024)
                : undefined
            "
            :controls="!zoom && !mobileViewer"
            playsinline
            preload="metadata"
            @error="
              error =
                'This browser cannot play this video. Try compatible playback below or download the original.'
            "
          />
          <img
            ref="photoImage"
            v-else-if="!previewFailed"
            :key="'image-' + asset.id"
            :src="photoSource"
            @load="imageLoaded"
            :alt="asset.filename"
            :style="{
              transform: `translate(${panX + swipeX}px, ${panY + swipeY}px) scale(${scale})`,
            }"
            :draggable="false"
            @dblclick="
              Date.now() - lastTouchAt > 600 &&
              (zoom
                ? resetZoom()
                : changeZoom(2, $event.clientX, $event.clientY))
            "
            @error="imageError"
          />
          <div v-else class="viewer-empty">
            <Icon name="photos" />
            <p>Full-resolution image unavailable</p>
            <small>The original is available to download.</small>
          </div>
          <div
            v-if="asset.mediaType === 'video' && (zoom || mobileViewer)"
            class="video-zoom-controls"
            @wheel.stop
            @pointerdown.stop
            @touchstart.stop
            @touchend.stop
          >
            <button
              :aria-label="playing ? 'Pause video' : 'Play video'"
              @click="togglePlayback"
            >
              <Icon :name="playing ? 'pause' : 'play'" />
            </button>
            <input
              type="range"
              min="0"
              :max="videoDuration || 1"
              step="0.1"
              :value="videoTime"
              :disabled="!videoDuration"
              aria-label="Video position"
              @input="seek"
            />
            <button v-if="zoom" @click="resetZoom">Fit</button>
          </div>
          <button
            class="viewer-prev icon-button"
            aria-label="Previous photo"
            :disabled="index === 0"
            @click="navigate(-1)"
          >
            <Icon name="left" /></button
          ><button
            class="viewer-next icon-button"
            aria-label="Next photo"
            :disabled="index === assets.length - 1"
            @click="navigate(1)"
          >
            <Icon name="right" />
          </button>
        </div>
        <aside v-if="info" class="metadata-panel">
          <AccessDetails :asset-id="asset.id" />
          <p v-if="asset.placeName" class="photo-place">
            <strong>Near {{ asset.placeName }}</strong
            ><small>Approximate location from photo GPS</small>
          </p>
          <h2>Details</h2>
          <p class="detail-date">
            {{
              new Date(asset.takenAt).toLocaleString(undefined, {
                dateStyle: "long",
                timeStyle: "short",
              })
            }}
          </p>
          <h3>
            {{
              asset.cameraModel ||
              asset.cameraMake ||
              "Camera information unavailable"
            }}
          </h3>
          <p>{{ asset.lens }}</p>
          <dl>
            <div v-if="asset.focalLength">
              <dt>Focal length</dt>
              <dd>{{ asset.focalLength }} mm</dd>
            </div>
            <div v-if="asset.aperture">
              <dt>Aperture</dt>
              <dd>f/{{ asset.aperture }}</dd>
            </div>
            <div v-if="asset.exposureTime">
              <dt>Exposure</dt>
              <dd>{{ asset.exposureTime }} s</dd>
            </div>
            <div v-if="asset.iso">
              <dt>ISO</dt>
              <dd>{{ asset.iso }}</dd>
            </div>
            <div>
              <dt>Dimensions</dt>
              <dd>{{ asset.width }} × {{ asset.height }}</dd>
            </div>
            <div>
              <dt>File size</dt>
              <dd>{{ (asset.fileSize / 1048576).toFixed(1) }} MB</dd>
            </div>
            <div v-if="asset.videoCodec">
              <dt>Video</dt>
              <dd>{{ asset.videoCodec }} · {{ asset.audioCodec }}</dd>
            </div>
          </dl>
          <p class="detail-path">{{ asset.relativePath }}</p>
          <p v-if="asset.error" class="processing-error">{{ asset.error }}</p>
          <a
            class="button"
            :href="`/api/assets/${asset.id}/original?download=1`"
            ><Icon name="download" />Download original</a
          ><button
            v-if="albumId && canEdit && asset.canRemove"
            class="button"
            @click="remove"
          >
            Remove from this album
          </button>
        </aside>
      </div>
      <div v-if="error" class="viewer-notice" role="status">
        {{ error }}
        <button aria-label="Dismiss notification" @click="error = ''">×</button>
      </div>
      <footer class="viewer-footer">
        <span v-if="asset.mediaType === 'video'" role="status">
          <template
            v-if="playbackState === 'pending' || playbackState === 'running'"
            >Preparing compatible video… You can close this window.</template
          >
          <template v-else>
            {{
              compatible ? "Compatible copy · H.264 / AAC" : "Original video"
            }}
            <button
              v-if="compatible"
              @click="
                resetPlayback();
                resetZoom();
              "
            >
              Use original
            </button>
            <button v-else @click="preparePlayback">
              {{
                playbackState === "failed"
                  ? "Retry compatible playback"
                  : "Compatible playback"
              }}
            </button>
          </template>
        </span>
        <span v-if="asset.mediaType !== 'video'" role="status">{{
          originalLoading
            ? "Loading full resolution…"
            : originalReady
              ? fullImageSource.endsWith("/original")
                ? "Original file · full resolution"
                : asset.mediaType === "raw"
                  ? "Developed RAW · full resolution"
                  : "Full resolution · lossless PNG"
              : "Full-resolution image unavailable"
        }}</span>
        <span>{{
          new Date(asset.takenAt).toLocaleDateString(undefined, {
            day: "numeric",
            month: "long",
            year: "numeric",
          })
        }}</span
        ><a :href="`/api/assets/${asset.id}/original?download=1`"
          >Download original <Icon name="download"
        /></a>
      </footer></div
  ></Teleport>
</template>

<style scoped>
.viewer-stage > video {
  transform-origin: center;
}
.viewer-stage.zoomed > video {
  max-width: 100%;
  max-height: 100%;
  cursor: grab;
}
.video-zoom-controls {
  position: absolute;
  bottom: 16px;
  left: 50%;
  transform: translateX(-50%);
  width: min(520px, 85%);
  display: flex;
  align-items: center;
  gap: 12px;
  z-index: 6;
  padding: 10px 14px;
  border-radius: 12px;
  background: #151917e8;
  color: #fff;
}
.video-zoom-controls button {
  color: inherit;
  background: transparent;
  border: 0;
  padding: 6px;
}
.video-zoom-controls input {
  flex: 1;
  min-width: 0;
  accent-color: #8fb6a5;
}
</style>

<style scoped>
@media (max-width: 700px) {
  .video-zoom-controls {
    bottom: calc(114px + env(safe-area-inset-bottom));
    width: calc(100% - 24px);
    padding: 6px 10px;
  }
  .controls-hidden .video-zoom-controls {
    opacity: 0;
    pointer-events: none;
  }
}
</style>
