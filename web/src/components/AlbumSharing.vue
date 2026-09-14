<script setup lang="ts">
import { ref, onMounted } from "vue";
import ShareSchedule from "./ShareSchedule.vue";
import { api } from "../api";
import type { Album } from "../types";
const props = defineProps<{ album: Album }>();
const emit = defineEmits<{ changed: [album: Album]; deleted: [] }>();
const open = ref(false),
  members = ref<
    {
      id: number;
      username: string;
      displayName: string;
      role: string;
      startsAt: number;
      expiresAt: number;
    }[]
  >([]),
  name = ref(props.album.name),
  description = ref(props.album.description),
  username = ref(""),
  role = ref("viewer"),
  error = ref(""),
  busy = ref(false),
  confirmDelete = ref(false);
type PublicShare = {
  startsAt: number;
  expiresAt: number;
  allowDownloads: boolean;
  createdAt: number;
};
const mode = ref<"users" | "public">("users"),
  publicShare = ref<PublicShare | null>(null),
  publicURL = ref(""),
  copied = ref(false),
  start = ref(""),
  end = ref(localTime(Date.now() + 7 * 86400000)),
  publicStart = ref(""),
  publicEnd = ref(localTime(Date.now() + 7 * 86400000)),
  allowDownloads = ref(false);
function localTime(ms: number) {
  const d = new Date(ms);
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
}
function iso(v: string) {
  return v ? new Date(v).toISOString() : "";
}
function timing(s: { startsAt: number; expiresAt: number }) {
  const now = Date.now() / 1000;
  if (s.expiresAt && s.expiresAt <= now) return "Expired";
  if (s.startsAt > now)
    return "Starts " + new Date(s.startsAt * 1000).toLocaleString();
  return s.expiresAt
    ? "Expires " + new Date(s.expiresAt * 1000).toLocaleString()
    : "No expiry";
}
async function createLink() {
  await run(async () => {
    const result = await api<{ path: string; share: PublicShare }>(
      `/albums/${props.album.id}/public-link`,
      "POST",
      {
        startsAt: iso(publicStart.value),
        expiresAt: iso(publicEnd.value),
        allowDownloads: allowDownloads.value,
      },
    );
    publicShare.value = result.share;
    publicURL.value = new URL(result.path, location.origin).href;
    copied.value = false;
  });
}
async function revokeLink() {
  await run(async () => {
    await api(`/albums/${props.album.id}/public-link`, "DELETE");
    publicShare.value = null;
    publicURL.value = "";
    copied.value = false;
  });
}
async function copyLink() {
  try {
    await navigator.clipboard.writeText(publicURL.value);
    copied.value = true;
  } catch {
    error.value = "Select the link and copy it manually.";
  }
}
function editMember(m: (typeof members.value)[number]) {
  username.value = m.username;
  role.value = m.role;
  start.value = m.startsAt ? localTime(m.startsAt * 1000) : "";
  end.value = m.expiresAt ? localTime(m.expiresAt * 1000) : "";
}
async function run(fn: () => Promise<void>) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    await fn();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function load() {
  members.value = (
    await api<{ items: typeof members.value }>(
      `/albums/${props.album.id}/members`,
    )
  ).items;
  publicShare.value = (
    await api<{ share: PublicShare | null }>(
      `/albums/${props.album.id}/public-link`,
    )
  ).share;
  if (publicShare.value) {
    publicStart.value = publicShare.value.startsAt
      ? localTime(publicShare.value.startsAt * 1000)
      : "";
    publicEnd.value = publicShare.value.expiresAt
      ? localTime(publicShare.value.expiresAt * 1000)
      : "";
    allowDownloads.value = publicShare.value.allowDownloads;
  }
}
async function share() {
  await run(async () => {
    await api(`/albums/${props.album.id}/members`, "POST", {
      username: username.value,
      role: role.value,
      startsAt: iso(start.value),
      expiresAt: iso(end.value),
    });
    username.value = "";
    await load();
  });
}
async function revoke(id: number) {
  await run(async () => {
    await api(`/albums/${props.album.id}/members/${id}`, "DELETE");
    await load();
  });
}
async function save() {
  await run(async () => {
    await api(`/albums/${props.album.id}`, "PATCH", {
      name: name.value,
      description: description.value,
    });
    emit("changed", {
      ...props.album,
      name: name.value,
      description: description.value,
    });
  });
}
async function remove() {
  await run(async () => {
    await api(`/albums/${props.album.id}`, "DELETE");
    emit("deleted");
  });
}
onMounted(() => {
  if (props.album.role === "owner") void run(load);
});
</script>
<template>
  <section class="album-sharing">
    <div class="settings-heading">
      <div>
        <span class="permission-pill">{{
          album.role === "owner"
            ? publicShare
              ? "Public link · " + timing(publicShare)
              : members.length
                ? "Shared with " + members.length
                : "Private album"
            : album.role === "viewer"
              ? "View only"
              : "Contributor"
        }}</span>
        <p>{{ album.description || "Collected by " + album.ownerName }}</p>
      </div>
      <button
        v-if="album.role === 'owner'"
        class="button"
        @click="open = !open"
      >
        {{ open ? "Close settings" : "Share & manage" }}
      </button>
    </div>
    <p v-if="album.role === 'contributor'">
      You can add photos from your assigned folders and remove your own
      additions.
    </p>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <div v-if="open && album.role === 'owner'" class="sharing-columns">
      <div>
        <div class="sharing-tabs" role="group" aria-label="Sharing audience">
          <button
            class="button"
            :aria-pressed="mode === 'users'"
            @click="mode = 'users'"
          >
            Selected users</button
          ><button
            class="button"
            :aria-pressed="mode === 'public'"
            @click="mode = 'public'"
          >
            Public link
          </button>
        </div>
        <form
          v-if="mode === 'users'"
          class="settings-form"
          @submit.prevent="share"
        >
          <h3>Invite to this album</h3>
          <p>
            Only selected signed-in users can open it. Sharing includes
            originals and downloads.
          </p>
          <label
            >Exact username<input
              v-model="username"
              required
              maxlength="64"
              placeholder="e.g. julia"
              autocomplete="off" /></label
          ><label
            >Permission<select v-model="role">
              <option value="viewer">Viewer · view and download</option>
              <option value="contributor">
                Contributor · also add own photos
              </option>
            </select></label
          ><ShareSchedule v-model:start="start" v-model:end="end" /><button
            class="primary"
            :disabled="busy"
          >
            Share / update access
          </button>
          <div v-for="m in members" :key="m.id" class="grant-row">
            <span
              ><strong>{{ m.displayName }}</strong
              ><small
                >@{{ m.username }} · {{ m.role }} · {{ timing(m) }}</small
              ></span
            ><button
              type="button"
              class="text-button"
              :disabled="busy"
              @click="editMember(m)"
            >
              Edit</button
            ><button
              type="button"
              class="text-button"
              :disabled="busy"
              @click="revoke(m.id)"
            >
              Revoke
            </button>
          </div>
          <p v-if="!members.length">Just yours for now.</p>
        </form>
        <form v-else class="settings-form" @submit.prevent="createLink">
          <h3>Anyone with the link</h3>
          <p>
            No account needed. Anyone receiving or forwarding this link can view
            this album's previews.
          </p>
          <p v-if="publicShare" class="permission-pill">
            {{ timing(publicShare) }} ·
            {{
              publicShare.allowDownloads
                ? "Original downloads allowed"
                : "Previews only"
            }}
          </p>
          <ShareSchedule
            v-model:start="publicStart"
            v-model:end="publicEnd"
          /><label class="inline-label"
            ><input type="checkbox" v-model="allowDownloads" />Allow original
            downloads and video playback</label
          >
          <p v-if="allowDownloads">
            Original files can include location and camera metadata. Downloaded
            copies cannot be recalled.
          </p>
          <button class="primary" :disabled="busy">
            {{
              publicShare
                ? "Replace link with these settings"
                : "Create public link"
            }}
          </button>
          <p v-if="publicShare">
            Replacing the link immediately invalidates the previous URL.
            Existing user invitations stay separate.
          </p>
          <div v-if="publicURL" class="public-link-result">
            <label
              >Public album link<input
                :value="publicURL"
                readonly
                @focus="($event.target as HTMLInputElement).select()" /></label
            ><button type="button" class="button" @click="copyLink">
              {{ copied ? "Copied" : "Copy link" }}</button
            ><a :href="publicURL" target="_blank" rel="noreferrer"
              >Open shared album</a
            >
          </div>
          <p v-else-if="publicShare">
            For privacy, the saved link cannot be retrieved. Replace it to
            obtain a new URL.
          </p>
          <button
            v-if="publicShare"
            type="button"
            class="text-button"
            :disabled="busy"
            @click="revokeLink"
          >
            Revoke public link
          </button>
        </form>
      </div>
      <form class="settings-form" @submit.prevent="save">
        <h3>Album details</h3>
        <label>Name<input v-model="name" required maxlength="120" /></label
        ><label
          >Description<textarea
            v-model="description"
            maxlength="2000"
            rows="3"
          ></textarea></label
        ><button class="button" :disabled="busy">Save details</button
        ><button
          type="button"
          class="text-button"
          @click="confirmDelete = !confirmDelete"
        >
          Delete album…
        </button>
        <div v-if="confirmDelete">
          <p>
            Delete this album and its sharing permissions? Your original photos
            stay in their folders.
          </p>
          <button type="button" class="button" :disabled="busy" @click="remove">
            Delete album
          </button>
        </div>
      </form>
    </div>
  </section>
</template>
