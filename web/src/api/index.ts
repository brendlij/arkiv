export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch("/api" + path, {
    method,
    signal,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-Gallery-Request": "1" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok)
    throw new ApiError(
      response.status,
      (await response.text()).trim() || "Request failed",
    );
  return response.status === 204 || response.status === 202
    ? (undefined as T)
    : response.json();
}
export const thumbnail = (id: number, size = 256) =>
  `/api/assets/${id}/thumbnail/${size}`;
