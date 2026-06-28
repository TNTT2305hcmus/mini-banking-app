import { ApiError } from "./api.service"

const ERROR_MESSAGES: Record<string, string> = {
  NETWORK_ERROR: "Không thể kết nối tới máy chủ. Vui lòng kiểm tra kết nối và thử lại.",
  INVALID_RESPONSE: "Máy chủ trả về dữ liệu không hợp lệ. Vui lòng thử lại sau.",
  UNKNOWN_ERROR: "Không thể hoàn tất yêu cầu. Vui lòng thử lại.",
  INTERNAL_ERROR: "Hệ thống đang gặp sự cố. Vui lòng thử lại sau.",

  INVALID_EMAIL: "Địa chỉ email không hợp lệ.",
  MISSING_REQUIRED_FIELD: "Vui lòng nhập đầy đủ thông tin bắt buộc.",
  INVALID_REQUEST: "Thông tin gửi lên chưa hợp lệ. Vui lòng kiểm tra và thử lại.",
  BAD_REQUEST: "Thông tin giao dịch chưa hợp lệ. Vui lòng kiểm tra lại.",
  INVALID_NONCE: "Yêu cầu xác thực không hợp lệ. Vui lòng thử đăng nhập lại.",
  INVALID_IV: "Dữ liệu bảo mật của giao dịch không hợp lệ. Vui lòng thử lại.",
  INVALID_PAGINATION: "Thông tin phân trang không hợp lệ.",

  INVALID_OTP_FORMAT: "Mã OTP phải gồm đúng 6 chữ số.",
  OTP_NOT_FOUND: "Mã OTP không tồn tại hoặc đã hết hạn. Vui lòng yêu cầu mã mới.",
  OTP_MISMATCH: "Mã OTP không chính xác. Vui lòng kiểm tra lại.",
  OTP_RATE_LIMITED: "Bạn đã yêu cầu OTP quá nhiều lần. Vui lòng chờ trước khi thử lại.",
  VERIFY_RATE_LIMITED: "Bạn đã nhập sai OTP quá nhiều lần. Vui lòng thử lại sau.",

  REG_TOKEN_EXPIRED: "Phiên đăng ký đã hết hạn. Vui lòng đăng ký lại từ đầu.",
  REG_TOKEN_INVALID: "Phiên đăng ký không hợp lệ. Vui lòng đăng ký lại từ đầu.",
  REG_TOKEN_USED: "Phiên đăng ký này đã được sử dụng. Vui lòng đăng ký lại từ đầu.",
  IDENTITY_ALREADY_REGISTERED: "Tài khoản này đã được đăng ký.",
  INVALID_CSR_FORMAT: "Không thể tạo yêu cầu cấp chứng chỉ. Vui lòng đăng ký lại.",
  CERT_NOT_FOUND: "Không tìm thấy chứng chỉ trên hệ thống. Vui lòng đăng ký lại thiết bị.",
  CERT_NOT_OWNED: "Chứng chỉ không thuộc về tài khoản hiện tại.",
  CERT_REVOKED: "Chứng chỉ đã bị thu hồi. Vui lòng liên hệ quản trị viên.",
  CERT_EXPIRED: "Chứng chỉ đã hết hạn. Vui lòng đăng ký lại thiết bị.",
  CERT_REJECTED: "Chứng chỉ không còn hợp lệ. Vui lòng liên hệ quản trị viên.",
  CA_SERVICE_UNAVAILABLE: "Dịch vụ chứng chỉ hiện không khả dụng. Vui lòng thử lại sau.",

  REQUEST_EXPIRED: "Yêu cầu đã hết thời gian hiệu lực. Vui lòng thử lại.",
  SIGNATURE_INVALID: "Không thể xác minh chữ ký số. Vui lòng đăng ký lại thiết bị.",
  INVALID_SIGNATURE: "Chữ ký giao dịch không hợp lệ. Vui lòng thử lại.",
  AUTH_INVALID: "Thông tin xác thực không hợp lệ. Vui lòng đăng nhập lại.",
  INVALID_AUTH_C: "Thông tin xác thực phiên không hợp lệ. Vui lòng đăng nhập lại.",
  INVALID_AUTHENTICATOR: "Phiên xác thực giao dịch không hợp lệ. Vui lòng đăng nhập lại.",
  UNAUTHENTICATED: "Phiên đăng nhập không hợp lệ hoặc đã hết hạn. Vui lòng đăng nhập lại.",
  REPLAY_DETECTED: "Yêu cầu bị trùng lặp và đã được hệ thống từ chối. Vui lòng thử lại.",
  STALE_REQUEST: "Yêu cầu đã quá hạn. Vui lòng thực hiện lại thao tác.",
  AUTH_RATE_LIMITED: "Bạn thao tác đăng nhập quá nhiều lần. Vui lòng thử lại sau.",
  KDC_UNAVAILABLE: "Dịch vụ xác thực hiện không khả dụng. Vui lòng thử lại sau.",

  INVALID_TGT_FORMAT: "Phiên đăng nhập không hợp lệ. Vui lòng đăng nhập lại.",
  TGT_EXPIRED: "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.",
  INVALID_TICKET: "Phiên giao dịch không hợp lệ hoặc đã hết hạn. Vui lòng đăng nhập lại.",
  SCOPE_DENIED: "Phiên hiện tại không có quyền thực hiện thao tác này.",
  WRONG_SCOPE: "Phiên giao dịch không có quyền chuyển khoản.",

  FORBIDDEN: "Bạn không có quyền thực hiện thao tác trên tài khoản này.",
  NOT_FOUND: "Không tìm thấy tài khoản được yêu cầu.",
  ACCOUNT_NOT_FOUND: "Không tìm thấy tài khoản được yêu cầu.",
  FROM_ACCOUNT_NOT_FOUND: "Không tìm thấy tài khoản nguồn.",
  TO_ACCOUNT_NOT_FOUND: "Không tìm thấy tài khoản nhận.",
  ACCOUNT_NOT_ACTIVE: "Tài khoản hiện không hoạt động nên không thể giao dịch.",
  INSUFFICIENT_FUNDS: "Số dư tài khoản không đủ để thực hiện giao dịch.",
  DAILY_LIMIT_EXCEEDED: "Giao dịch vượt quá hạn mức chuyển tiền trong ngày.",
  IDEMPOTENCY_FAILED: "Giao dịch này đã được gửi trước đó. Vui lòng kiểm tra lịch sử giao dịch.",
  ALREADY_EXISTS: "Dữ liệu này đã tồn tại trên hệ thống.",
  UNPROCESSABLE_ENTITY: "Không thể thực hiện giao dịch với trạng thái hiện tại.",
  RATE_LIMITED: "Bạn thao tác quá nhanh. Vui lòng chờ rồi thử lại.",
  BANK_RATE_LIMITED: "Bạn thực hiện quá nhiều yêu cầu ngân hàng. Vui lòng thử lại sau.",
  SERVICE_UNAVAILABLE: "Dịch vụ ngân hàng hiện không khả dụng. Vui lòng thử lại sau.",
}

const normalizeCode = (value: string) => value.trim().toUpperCase().replace(/[\s-]+/g, "_")

const messageByStatus = (status: number): string | undefined => {
  if (status === 400) return "Thông tin gửi lên chưa hợp lệ. Vui lòng kiểm tra lại."
  if (status === 401) return "Phiên xác thực không hợp lệ hoặc đã hết hạn. Vui lòng đăng nhập lại."
  if (status === 403) return "Bạn không có quyền thực hiện thao tác này."
  if (status === 404) return "Không tìm thấy dữ liệu được yêu cầu."
  if (status === 408) return "Yêu cầu đã hết thời gian xử lý. Vui lòng thử lại."
  if (status === 409) return "Yêu cầu bị trùng với dữ liệu đã tồn tại."
  if (status === 422) return "Không thể thực hiện yêu cầu với trạng thái hiện tại."
  if (status === 429) return "Bạn thao tác quá nhiều lần. Vui lòng thử lại sau."
  if (status >= 500) return "Dịch vụ đang gặp sự cố. Vui lòng thử lại sau."
  return undefined
}

/** Chuyển lỗi kỹ thuật thành thông báo có thể hiển thị trực tiếp cho người dùng. */
export function getUserErrorMessage(
  error: unknown,
  fallback = "Có lỗi xảy ra. Vui lòng thử lại.",
): string {
  if (error instanceof ApiError) {
    // Bank gRPC hiện có thể bọc mã nghiệp vụ bằng một error_code HTTP chung
    // (vd. code=UNPROCESSABLE_ENTITY, message=INSUFFICIENT_FUNDS).
    // Chỉ đọc message như code khi nó thực sự có định dạng CONSTANT_CASE để
    // không hiển thị hoặc suy diễn từ thông tin kỹ thuật tự do của server.
    const serverCode = /^[A-Z][A-Z0-9_-]*$/.test(error.message.trim())
      ? normalizeCode(error.message)
      : ""
    return (
      (serverCode ? ERROR_MESSAGES[serverCode] : undefined) ??
      ERROR_MESSAGES[normalizeCode(error.code)] ??
      messageByStatus(error.status) ??
      fallback
    )
  }

  if (error instanceof Error) {
    const mapped = ERROR_MESSAGES[normalizeCode(error.message)]
    if (mapped) return mapped
  }

  return fallback
}
