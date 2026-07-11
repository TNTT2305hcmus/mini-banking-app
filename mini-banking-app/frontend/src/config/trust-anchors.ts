// Trust anchor (Root CA) — ĐỂ NGOÀI, không hard-code, nạp lúc frontend chạy.
//
// Nguồn: file tĩnh `public/trust/root-ca.pem` → phục vụ tại `/trust/root-ca.pem`.
// Có thể đổi vị trí qua env VITE_ROOT_CA_URL lúc build.
//
// ⚠️ RÀNG BUỘC BẢO MẬT (bắt buộc):
//   - File này PHẢI được phục vụ bởi CHÍNH origin của frontend (cùng nơi phát hành
//     bundle SPA), qua HTTPS. TUYỆT ĐỐI KHÔNG lấy Root CA từ API Gateway hay bất kỳ
//     bên nào có thể tạo AS_REP — nếu không, kẻ tấn công kiểm soát kênh đó vừa cấp
//     anchor giả vừa cấp AS_REP giả → vô hiệu hóa toàn bộ xác minh.
//   - Trong dev, Vite chỉ proxy `/v1` sang gateway; `/trust/root-ca.pem` do chính
//     Vite (origin frontend) phục vụ → đúng yêu cầu. Khi deploy, đặt file cạnh SPA
//     trên host tĩnh của frontend, KHÔNG sau gateway.
//   - Fail-closed: nếu không nạp được anchor, mọi verify sẽ ném lỗi (không có phiên).

// URL nơi Root CA PEM được phục vụ. Mặc định same-origin của SPA.
function rootCaUrl(): string {
  // Vite thay thế import.meta.env.* lúc build; Node/test không có → fallback an toàn.
  const fromEnv = (import.meta as unknown as { env?: Record<string, string | undefined> }).env
    ?.VITE_ROOT_CA_URL;
  return fromEnv && fromEnv.trim() ? fromEnv.trim() : "/trust/root-ca.pem";
}

export const ROOT_CA_URL = rootCaUrl();

// Cho phép nạp trực tiếp PEM (test/SSR) thay vì fetch qua mạng.
let overridePem: string | null = null;
let pemPromise: Promise<string> | null = null;

export function setRootCaPem(pem: string): void {
  overridePem = pem;
  pemPromise = null;
}

// Nạp Root CA PEM (fetch một lần, cache). Ném nếu không tải được / PEM không hợp lệ.
export function loadRootCaPem(): Promise<string> {
  if (overridePem !== null) return Promise.resolve(overridePem);
  if (!pemPromise) {
    pemPromise = fetch(ROOT_CA_URL, { cache: "force-cache" })
      .then(async (res) => {
        if (!res.ok) {
          throw new Error(`Không tải được Root CA từ ${ROOT_CA_URL} (HTTP ${res.status})`);
        }
        const text = await res.text();
        if (!text.includes("BEGIN CERTIFICATE")) {
          throw new Error(`Root CA tại ${ROOT_CA_URL} không phải PEM hợp lệ`);
        }
        return text;
      })
      .catch((err) => {
        pemPromise = null; // reset để lần verify sau thử lại (vẫn fail-closed ở nơi gọi)
        throw err;
      });
  }
  return pemPromise;
}
