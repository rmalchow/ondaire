// Module-scoped reactive toast list (J arch §4 Toast). api.js pushes on error;
// Toast.svelte renders. Auto-dismiss handled in the component.

let nextId = 1;

// toasts is the reactive list of {id, msg, kind}. kind: "error" | "ok".
export const toasts = $state({ list: [] });

// pushToast appends a toast and returns its id.
export function pushToast(msg, kind = "error") {
  const id = nextId++;
  toasts.list.push({ id, msg: String(msg), kind });
  return id;
}

// dismissToast removes a toast by id.
export function dismissToast(id) {
  const i = toasts.list.findIndex((t) => t.id === id);
  if (i >= 0) toasts.list.splice(i, 1);
}
