<script setup lang="ts">
import { ref, onMounted } from "vue";
import { api } from "../api";
import type { User, Folder } from "../types";
defineProps<{ currentId: number }>();
const users = ref<User[]>([]),
  selected = ref<User>(),
  libraries = ref<{ id: string; name: string }[]>([]),
  grants = ref<(Folder & { id: number })[]>([]),
  error = ref(""),
  notice = ref(""),
  busy = ref(false),
  creating = ref(false);
const storage = ref<{
  quota: number;
  used: number;
  reserved: number;
  override: number;
}>();
const quotaGiB = ref(0);
const username = ref(""),
  name = ref(""),
  role = ref("member"),
  password = ref(""),
  resetPassword = ref(""),
  libraryId = ref(""),
  folder = ref(""),
  label = ref("");
async function action(fn: () => Promise<void>) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await fn();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function load() {
  users.value = (await api<{ items: User[] }>("/admin/users")).items;
}
async function select(u: User) {
  selected.value = { ...u };
  storage.value = await api(`/admin/users/${u.id}/storage`);
  quotaGiB.value = (storage.value?.override || 0) / 1073741824;
  resetPassword.value = "";
  grants.value = (
    await api<{ items: (Folder & { id: number })[] }>(
      `/admin/users/${u.id}/folders`,
    )
  ).items;
}
async function saveStorage() {
  await action(async () => {
    storage.value = await api(
      `/admin/users/${selected.value!.id}/storage`,
      "POST",
      { quotaBytes: Math.round(quotaGiB.value * 1073741824) },
    );
    notice.value =
      "Storage limit updated. Existing files and reserved transfers are preserved.";
  });
}
async function create() {
  await action(async () => {
    await api("/admin/users", "POST", {
      username: username.value,
      displayName: name.value,
      role: role.value,
      password: password.value,
    });
    password.value = "";
    username.value = name.value = "";
    creating.value = false;
    notice.value =
      "Account created. Share the temporary password privately; the user must replace it at first sign-in.";
    await load();
  });
}
async function save() {
  await action(async () => {
    await api(`/admin/users/${selected.value!.id}`, "PATCH", selected.value);
    notice.value = "Account updated; existing sessions were signed out.";
    await load();
  });
}
async function reset() {
  await action(async () => {
    await api(`/admin/users/${selected.value!.id}/password`, "POST", {
      password: resetPassword.value,
    });
    resetPassword.value = "";
    notice.value = "Temporary password set. All sessions have been revoked.";
    await load();
  });
}
async function grant() {
  await action(async () => {
    await api(`/admin/users/${selected.value!.id}/folders`, "POST", {
      libraryId: libraryId.value,
      folder: folder.value,
      label: label.value,
    });
    await select(selected.value!);
    folder.value = label.value = "";
    notice.value = "Folder assigned, including subfolders.";
  });
}
async function revoke(id: number) {
  await action(async () => {
    await api(`/admin/users/${selected.value!.id}/folders/${id}`, "DELETE");
    await select(selected.value!);
    notice.value = "Folder access revoked.";
  });
}
onMounted(() =>
  action(async () => {
    await load();
    libraries.value = (
      await api<{ items: { id: string; name: string }[] }>("/admin/libraries")
    ).items;
    libraryId.value = libraries.value[0]?.id || "";
  }),
);
</script>
<template>
  <section class="people-settings" :class="{ 'has-selection': selected }">
    <div class="settings-card">
      <div class="settings-heading">
        <div>
          <h2>A place for everyone</h2>
          <p>
            Create an account, assign folders, then let people share their
            stories.
          </p>
        </div>
        <button class="primary" @click="creating = !creating">
          Add person
        </button>
      </div>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p v-if="notice" role="status">{{ notice }}</p>
      <form v-if="creating" class="settings-form" @submit.prevent="create">
        <label
          >Username<input
            v-model="username"
            required
            minlength="2"
            maxlength="64"
            pattern="[A-Za-z0-9][A-Za-z0-9_.-]+"
            autocomplete="off" /></label
        ><label
          >Display name<input v-model="name" required maxlength="120" /></label
        ><label
          >Role<select v-model="role">
            <option value="member">Member · assigned folders</option>
            <option value="admin">
              Administrator · all libraries and albums
            </option>
          </select></label
        ><label
          >Temporary password<input
            v-model="password"
            required
            type="password"
            minlength="12"
            maxlength="1024"
            autocomplete="new-password" /></label
        ><button class="primary" :disabled="busy">Create account</button>
      </form>
      <div class="people-list">
        <button
          v-for="u in users"
          :key="u.id"
          class="person-row"
          :class="{ selected: selected?.id === u.id }"
          @click="action(() => select(u))"
        >
          <span class="avatar">{{
            u.displayName.slice(0, 1).toUpperCase()
          }}</span
          ><span
            ><strong>{{ u.displayName }}</strong
            ><small
              >@{{ u.username }} · {{ u.role
              }}{{ !u.enabled ? " · Disabled" : "" }}</small
            ></span
          ><span class="permission-pill">{{
            !u.enabled
              ? "Disabled"
              : u.mustChangePassword
                ? "Setup pending"
                : "Active"
          }}</span>
        </button>
      </div>
    </div>
    <div v-if="selected" class="settings-card">
      <h2>{{ selected.displayName }}</h2>
      <form class="settings-form" @submit.prevent="save">
        <label
          >Display name<input
            v-model="selected.displayName"
            required
            maxlength="120" /></label
        ><label
          >Role<select
            v-model="selected.role"
            :disabled="selected.id === currentId"
          >
            <option value="member">Member</option>
            <option value="admin">Administrator</option>
          </select></label
        ><label class="inline-label"
          ><input
            type="checkbox"
            v-model="selected.enabled"
            :disabled="selected.id === currentId"
          />Account enabled</label
        ><button class="button" :disabled="busy">Save account</button>
      </form>
      <h3>Account upload quota</h3>
      <p v-if="storage">
        {{ (storage.used / 1073741824).toFixed(2) }} GiB used +
        {{ (storage.reserved / 1073741824).toFixed(2) }} GiB reserved /
        {{ (storage.quota / 1073741824).toFixed(2) }} GiB limit.
      </p>
      <form class="settings-form" @submit.prevent="saveStorage">
        <label
          >Per-user limit (GiB; 0 uses server default)<input
            v-model.number="quotaGiB"
            type="number"
            min="0"
            max="1048576"
            step="0.01"
            required /></label
        ><button class="button" :disabled="busy">Save account quota</button>
      </form>
      <h3>Assigned folders</h3>
      <p>
        {{
          selected.role === "admin"
            ? "Administrators already have access to every configured library."
            : "Access includes all subfolders. Assigned server libraries remain read-only."
        }}
      </p>
      <div v-for="g in grants" :key="g.id" class="grant-row">
        <span
          ><strong>{{ g.name }}</strong
          ><small>{{ g.libraryId }} / {{ g.path || "(root)" }}</small></span
        ><button class="text-button" :disabled="busy" @click="revoke(g.id)">
          Revoke
        </button>
      </div>
      <form class="settings-form" @submit.prevent="grant">
        <label
          >Library<select v-model="libraryId" required>
            <option v-for="l in libraries" :key="l.id" :value="l.id">
              {{ l.name }}
            </option>
          </select></label
        ><label
          >Relative folder<input
            v-model="folder"
            placeholder="Family/Julia (empty for whole library)" /></label
        ><label
          >Personal label<input
            v-model="label"
            placeholder="My photos"
            required
            maxlength="120" /></label
        ><button class="button" :disabled="busy || !libraryId">
          Assign folder
        </button>
      </form>
      <form
        v-if="selected.id !== currentId"
        class="settings-form"
        @submit.prevent="reset"
      >
        <h3>Reset sign-in</h3>
        <label
          >New temporary password<input
            v-model="resetPassword"
            required
            type="password"
            minlength="12"
            maxlength="1024"
            autocomplete="new-password" /></label
        ><button class="button" :disabled="busy">
          Reset password and sign out user
        </button>
      </form>
    </div>
  </section>
</template>
