export interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  headers?: Record<string, string>;
  body?: unknown;
  token?: string;
  signal?: AbortSignal;
}
export class ApiError extends Error {
  readonly status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}
export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
export function errorMessage(error: unknown): string {
  return isRecord(error) && typeof error.message === "string" ? error.message : "";
}
export function errorStatus(error: unknown): number | undefined {
  return isRecord(error) && typeof error.status === "number" ? error.status : undefined;
}
// Data endpoints require a JSON body; commands may legitimately return 204/null.
export async function apiData<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const data = await apiRequest<T>(path, options);
  if (data === null) throw new Error("服务器未返回数据");
  return data;
}

export async function apiRequest<T = unknown>(path: string, options: RequestOptions = {}): Promise<T | null> {
  const headers: Record<string, string> = {
    Accept: "application/json",
    ...(options.headers || {})
  };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (options.token) headers.Authorization = `Bearer ${options.token}`;

  const response = await fetch(path, {
    method: options.method || "GET",
    headers,
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
    signal: options.signal
  });

  if (!response.ok) {
    let message = "请求失败";
    try {
      const data: unknown = await response.json();
      if (isRecord(data) && typeof data.error === "string" && data.error) message = data.error;
      if (isRecord(data) && typeof data.message === "string" && data.message) message = data.message;
    } catch {
      message = response.statusText || message;
    }
    throw new ApiError(message, response.status);
  }

  if (response.status === 204) return null;
  return response.json();
}

export async function uploadFile(file: File, kind: "video" | "cover" | "avatar", token: string, signal?: AbortSignal): Promise<{ url: string }> {
  const data = new FormData();
  data.append("file", file);
  data.append("kind", kind);

  const response = await fetch("/api/uploads", {
    method: "POST",
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${token}`
    },
    body: data,
    signal
  });

  if (!response.ok) {
    let message = "上传失败";
    try {
      const payload: unknown = await response.json();
      if (isRecord(payload) && typeof payload.error === "string" && payload.error) message = payload.error;
      if (isRecord(payload) && typeof payload.message === "string" && payload.message) message = payload.message;
    } catch {
      message = response.statusText || message;
    }
    throw new Error(message);
  }

  return response.json();
}
