// HTTP client dùng chung cho gateway. Tự gắn X-Request-ID, parse envelope
// { success, data, error_code, message } và ném ApiError khi thất bại.

// Base URL: mặc định rỗng (dùng proxy /v1 của Vite ở dev), override bằng VITE_API_BASE_URL
const BASE_URL = ((import.meta.env.VITE_API_BASE_URL as string) ?? "").replace(/\/$/, "");

// Lỗi chuẩn hóa từ gateway (giữ error_code và HTTP status)
export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

// Shape envelope của gateway
interface ApiEnvelope<T> {
  success: boolean;
  data?: T;
  message?: string;
  error_code?: string;
}

// Gửi POST JSON và trả về phần data; ném ApiError khi network/HTTP/envelope lỗi
export async function apiPost<T>(
  path: string,
  body: unknown,
  headers: Record<string, string> = {},
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${BASE_URL}${path}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Request-ID": crypto.randomUUID(),
        ...headers,
      },
      body: JSON.stringify(body),
    });
  } catch {
    throw new ApiError("NETWORK_ERROR", "Không thể kết nối tới máy chủ", 0);
  }

  let payload: ApiEnvelope<T>;
  try {
    payload = await res.json();
  } catch {
    throw new ApiError("INVALID_RESPONSE", `Phản hồi không hợp lệ (HTTP ${res.status})`, res.status);
  }

  if (!res.ok || !payload.success) {
    throw new ApiError(
      payload.error_code ?? "UNKNOWN_ERROR",
      payload.message ?? `Yêu cầu thất bại (HTTP ${res.status})`,
      res.status,
    );
  }

  return payload.data as T;
}

export async function apiGet<T>(
  path: string,
  headers: Record<string, string> = {},
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${BASE_URL}${path}`, {
      method: "GET",
      headers: {
        "X-Request-ID": crypto.randomUUID(),
        ...headers,
      },
    });
  } catch {
    throw new ApiError("NETWORK_ERROR", "Khong the ket noi toi may chu", 0);
  }

  let payload: ApiEnvelope<T>;
  try {
    payload = await res.json();
  } catch {
    throw new ApiError("INVALID_RESPONSE", `Phan hoi khong hop le (HTTP ${res.status})`, res.status);
  }

  if (!res.ok || !payload.success) {
    throw new ApiError(
      payload.error_code ?? "UNKNOWN_ERROR",
      payload.message ?? `Yeu cau that bai (HTTP ${res.status})`,
      res.status,
    );
  }

  return payload.data as T;
}
