<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, nextTick } from "vue";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { api, thumbnail } from "../api";
import type { Asset, Page } from "../types";
import Icon from "./Icon.vue";
import AssetViewer from "./AssetViewer.vue";
type Point = {
  lat: number;
  lon: number;
  count: number;
  assetId: number;
  south: number;
  west: number;
  north: number;
  east: number;
  name: string;
};
type Place = {
  id: string;
  name: string;
  count: number;
  lat: number;
  lon: number;
};
const host = ref<HTMLElement>(),
  places = ref<Place[]>([]),
  search = ref(""),
  error = ref(""),
  mapError = ref(""),
  geotagged = ref(0),
  withoutGPS = ref(0),
  truncated = ref(false),
  tiles = ref(localStorage.getItem("arkiv-map-mode") !== "offline"),
  selectedPlace = ref(""),
  tileError = ref(""),
  photoTitle = ref(""),
  photos = ref<Asset[]>([]),
  loading = ref(false),
  viewer = ref<number | null>(null),
  placeMore = ref(false),
  placeOffset = ref(0),
  nextCursor = ref(""),
  page = ref(0),
  cursors = ref([""]);
let map: L.Map | undefined,
  markers: L.LayerGroup | undefined,
  tileLayer: L.TileLayer | undefined,
  resize: ResizeObserver | undefined,
  debounce: ReturnType<typeof setTimeout> | undefined,
  searchTimer: ReturnType<typeof setTimeout> | undefined,
  request = 0,
  photoRequest = 0,
  placeRequest = 0,
  filter: Record<string, string> = {},
  initial = true,
  disposed = false;
let refreshTimer: ReturnType<typeof setTimeout> | undefined;
const refreshing = ref(false);
async function refreshMap() {
  if (refreshing.value || disposed) return;
  refreshing.value = true;
  try {
    await Promise.all([points(), loadPlaces(false)]);
  } finally {
    refreshing.value = false;
  }
}
function scheduleRefresh() {
  refreshTimer = setTimeout(async () => {
    if (!document.hidden) await refreshMap();
    if (!disposed) scheduleRefresh();
  }, 5000);
}
function overview() {
  void points(true);
}

function bbox() {
  const b = map!.getBounds();
  return [
    Math.max(-180, b.getWest()),
    Math.max(-90, b.getSouth()),
    Math.min(180, b.getEast()),
    Math.min(90, b.getNorth()),
  ].join(",");
}
async function points(fitAll = false) {
  if (!map) return;
  const current = ++request;
  try {
    const result = await api<{
      items: Point[];
      hasMore: boolean;
      geotagged: number;
      withoutGPS: number;
    }>(
      `/map?${new URLSearchParams({ ...(initial || fitAll ? {} : { bbox: bbox() }), zoom: String(map.getZoom()) })}`,
    );
    if (disposed || current !== request) return;
    geotagged.value = result.geotagged;
    withoutGPS.value = result.withoutGPS;
    truncated.value = result.hasMore;
    markers!.clearLayers();
    for (const p of result.items) {
      const title =
        (p.name ? "Near " + p.name : "GPS location") +
        " · " +
        p.count +
        " photos";
      const pin = document.createElement("div");
      pin.className = "map-photo-pin";
      const photo = document.createElement("img");
      photo.src = thumbnail(p.assetId);
      photo.alt = "";
      photo.loading = "lazy";
      photo.onerror = () => photo.remove();
      const count = document.createElement("span");
      count.textContent = String(p.count);
      pin.append(photo, count);
      const marker = L.marker([p.lat, p.lon], {
        icon: L.divIcon({
          className: "map-photo-marker",
          html: pin,
          iconSize: [56, 50],
          iconAnchor: [28, 50],
        }),
        title,
        keyboard: true,
      });
      const label = document.createElement("span");
      label.textContent = title;
      marker.bindTooltip(label).on("click", () => selectPoint(p));
      markers!.addLayer(marker);
    }
    if ((initial || fitAll) && result.items.length) {
      initial = false;
      const bounds = L.latLngBounds(
        result.items.flatMap(
          (p) =>
            [
              [p.south, p.west],
              [p.north, p.east],
            ] as L.LatLngTuple[],
        ),
      );
      map.fitBounds(bounds, { padding: [55, 55], maxZoom: 12 });
    }
    if (tiles.value && !tileLayer) changeTiles();
    mapError.value = "";
  } catch (e) {
    if (current === request && !disposed) {
      mapError.value = (e as Error).message;
      markers?.clearLayers();
    }
  }
}
async function loadPlaces(reset = true) {
  if (reset) placeOffset.value = 0;
  const current = ++placeRequest;
  try {
    const result = await api<{ items: Place[]; hasMore: boolean }>(
      `/places?${new URLSearchParams({ q: search.value, offset: String(placeOffset.value) })}`,
    );
    if (disposed || current !== placeRequest) return;
    error.value = "";
    places.value = result.items;
    placeMore.value = result.hasMore;
  } catch (e) {
    if (!disposed) {
      error.value = (e as Error).message;
      places.value = [];
    }
  }
}
async function loadPhotos(reset = true) {
  if (reset) {
    page.value = 0;
    cursors.value = [""];
  }
  const current = ++photoRequest;
  loading.value = true;
  error.value = "";
  viewer.value = null;
  try {
    const data = await api<Page>(
      `/assets?${new URLSearchParams({ ...filter, limit: "120", cursor: cursors.value[page.value] })}`,
    );
    if (disposed || current !== photoRequest) return;
    photos.value = data.items;
    nextCursor.value = data.nextCursor;
  } catch (e) {
    if (current === photoRequest && !disposed) {
      photos.value = [];
      error.value = (e as Error).message;
    }
  } finally {
    if (current === photoRequest && !disposed) loading.value = false;
  }
}
function selectPlace(p: Place) {
  selectedPlace.value = p.id;
  filter = { place: p.id };
  photoTitle.value = "Near " + p.name;
  map?.setView([p.lat, p.lon], 12);
  void loadPhotos();
}
function selectPoint(p: Point) {
  selectedPlace.value = "";
  const epsilon = 0.0000001;
  filter = {
    bbox: [
      Math.max(-180, p.west - epsilon),
      Math.max(-90, p.south - epsilon),
      Math.min(180, p.east + epsilon),
      Math.min(90, p.north + epsilon),
    ].join(","),
  };
  photoTitle.value = p.name ? "Near " + p.name : "Photos at this location";
  void loadPhotos();
  if (p.count > 1)
    map?.fitBounds(
      [
        [p.south, p.west],
        [p.north, p.east],
      ],
      { padding: [60, 60], maxZoom: 15 },
    );
}
function area() {
  selectedPlace.value = "";
  filter = { bbox: bbox() };
  photoTitle.value = "Photos in this map area";
  void loadPhotos();
}
function next() {
  cursors.value[page.value + 1] = nextCursor.value;
  page.value++;
  void loadPhotos(false);
}
function previous() {
  page.value--;
  void loadPhotos(false);
}
function favorite(a: Asset) {
  const i = photos.value.findIndex((p) => p.id === a.id);
  if (i >= 0) photos.value[i] = a;
}
function changeTiles() {
  if (!map) return;
  localStorage.setItem("arkiv-map-mode", tiles.value ? "street" : "offline");
  tileLayer?.remove();
  tileLayer = undefined;
  tileError.value = "";
  if (tiles.value) {
    tileLayer = L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
      maxZoom: 18,
      noWrap: true,
      referrerPolicy: "strict-origin-when-cross-origin",
      attribution:
        '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    })
      .on("tileerror", () => {
        tileError.value =
          "Street tiles unavailable. The offline map and photo markers still work.";
      })
      .addTo(map);
  } else {
    mapError.value = "";
  }
}
watch(search, () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => void loadPlaces(), 250);
});
onMounted(async () => {
  await nextTick();
  if (!host.value) return;
  map = L.map(host.value, {
    zoomControl: false,
    minZoom: 1,
    maxZoom: 18,
    maxBounds: [
      [-90, -180],
      [90, 180],
    ],
    maxBoundsViscosity: 1,
    worldCopyJump: false,
  }).setView([20, 0], 2);
  L.control.zoom({ position: "bottomright" }).addTo(map);
  map.createPane("offline-base").style.zIndex = "150";
  markers = L.layerGroup().addTo(map);
  map.attributionControl.addAttribution(
    '<a href="https://www.naturalearthdata.com/">Natural Earth</a>',
  );
  map.on("moveend", () => {
    clearTimeout(debounce);
    debounce = setTimeout(() => void points(), 200);
  });
  resize = new ResizeObserver(() => map?.invalidateSize());
  resize.observe(host.value);
  void points();
  void loadPlaces();
  scheduleRefresh();
  try {
    const result = await fetch("/maps/world.geojson");
    if (!result.ok) throw Error("Offline map unavailable");
    const world = await result.json();
    if (disposed) return;
    L.geoJSON(world, {
      pane: "offline-base",
      style: {
        color: "#9daf94",
        weight: 1,
        fillColor: "#dbe4d0",
        fillOpacity: 1,
      },
      interactive: false,
    })
      .addTo(map)
      .bringToBack();
  } catch (e) {
    if (!disposed) mapError.value = (e as Error).message;
  }
});
onUnmounted(() => {
  disposed = true;
  clearTimeout(refreshTimer);
  ++request;
  ++photoRequest;
  ++placeRequest;
  clearTimeout(debounce);
  clearTimeout(searchTimer);
  resize?.disconnect();
  map?.remove();
  map = undefined;
});
</script>
<template>
  <section class="map-browser map-explorer">
    <div class="map-explorer-bar">
      <span
        ><strong>{{ geotagged.toLocaleString() }}</strong> photos on the map
        <span class="map-missing"
          >· {{ withoutGPS }} without location</span
        ></span
      >
      <div class="map-tools">
        <button class="button" :disabled="refreshing" @click="refreshMap">
          <Icon name="refresh" />{{ refreshing ? "Refreshing…" : "Refresh" }}
        </button>
        <button class="button" @click="overview">
          <Icon name="expand" />All locations
        </button>
        <label class="map-mode"
          ><span class="sr-only">Map style</span
          ><select v-model="tiles" @change="changeTiles">
            <option :value="true">Street map</option>
            <option :value="false">Offline map</option>
          </select></label
        >
      </div>
    </div>
    <div class="map-layout">
      <aside class="places-panel">
        <header>
          <h2>Your places</h2>
          <p>Updates automatically as photos are processed.</p>
        </header>
        <label class="place-search"
          ><Icon name="search" /><input
            v-model="search"
            type="search"
            aria-label="Search your places"
            placeholder="Search places…"
            maxlength="200"
        /></label>
        <div class="place-results">
          <button
            v-for="p in places"
            :key="p.id"
            class="place-row"
            :class="{ selected: selectedPlace === p.id }"
            :aria-pressed="selectedPlace === p.id"
            @click="selectPlace(p)"
          >
            <Icon name="map" /><span
              ><strong>{{ p.name.split(",")[0] }}</strong
              ><small>{{
                p.name.split(",").slice(1).join(",").trim()
              }}</small></span
            ><span class="place-count">{{ p.count }}</span>
          </button>
          <p v-if="!places.length" class="places-empty">
            {{
              search
                ? "No matching places."
                : "Photos with GPS appear here after indexing."
            }}
          </p>
        </div>
        <div v-if="placeOffset || placeMore" class="pagination">
          <button
            class="button"
            :disabled="!placeOffset"
            @click="
              placeOffset -= 100;
              loadPlaces(false);
            "
          >
            Previous
          </button>
          <button
            class="button"
            :disabled="!placeMore"
            @click="
              placeOffset += 100;
              loadPlaces(false);
            "
          >
            More
          </button>
        </div>
        <small class="places-credit"
          >Nearby places ·
          <a href="https://www.geonames.org/" target="_blank" rel="noreferrer"
            >GeoNames</a
          >
          ·
          <a
            href="https://creativecommons.org/licenses/by/4.0/"
            target="_blank"
            rel="noreferrer"
            >CC BY 4.0</a
          ></small
        >
      </aside>
      <div class="map-main">
        <div
          ref="host"
          class="photo-map"
          aria-label="Photo locations map"
        ></div>
        <button class="map-area-button" @click="area">
          <Icon name="search" />Photos in this area
        </button>
        <div
          v-if="mapError || tileError || truncated"
          class="map-message"
          role="status"
        >
          {{ mapError || tileError || "Zoom in to see more photo locations." }}
        </div>
      </div>
    </div>
    <div class="map-caption">
      <span>{{
        tiles
          ? "Street tiles by OpenStreetMap; your IP and viewed area are sent to the provider."
          : "Offline world outline. No external map requests."
      }}</span
      ><span>Place labels are approximate.</span>
    </div>
    <section class="map-photos" aria-live="polite">
      <template v-if="photoTitle">
        <header class="map-selection-heading">
          <div>
            <span class="eyebrow">SELECTED LOCATION</span>
            <h2>{{ photoTitle }}</h2>
          </div>
          <button
            class="icon-button"
            aria-label="Clear location selection"
            @click="
              photoTitle = '';
              selectedPlace = '';
              photos = [];
              ++photoRequest;
            "
          >
            <Icon name="close" />
          </button>
        </header>
        <p v-if="loading">Loading photos…</p>
        <div v-else class="map-photo-rail">
          <button
            v-for="(photo, index) in photos"
            :key="photo.id"
            class="map-photo-card"
            :aria-label="`Open ${photo.filename}`"
            @click="viewer = index"
          >
            <img
              v-if="photo.previewStatus === 'ready'"
              :src="thumbnail(photo.id)"
              :alt="photo.filename"
              loading="lazy"
            /><span v-else>Preparing preview</span>
          </button>
        </div>
        <p v-if="!loading && !photos.length">
          No accessible photos in this selection.
        </p>
        <div v-if="page || nextCursor" class="pagination">
          <button
            class="button"
            :disabled="page === 0 || loading"
            @click="previous"
          >
            Previous</button
          ><span>Page {{ page + 1 }}</span
          ><button
            class="button"
            :disabled="!nextCursor || loading"
            @click="next"
          >
            Next
          </button>
        </div>
      </template>
      <div v-else class="map-selection-hint">
        <Icon name="photos" /><span
          >Choose a place or a photo on the map to explore.</span
        >
      </div>
    </section>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <AssetViewer
      v-if="viewer !== null && photos[viewer]"
      :assets="photos"
      :index="viewer"
      @close="viewer = null"
      @navigate="viewer = $event"
      @favorite="favorite"
    />
  </section>
</template>
