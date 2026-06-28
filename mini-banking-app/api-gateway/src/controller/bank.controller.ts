// Controller Bank: nhận request đã validate từ route và gọi Bank gRPC service.
import { Request, Response, NextFunction } from "express";
import {
  getBalance,
  getHistory,
  transferMoney,
} from "../services/bank.service";

// Xử lý chuyển tiền: decode opaque fields rồi gọi BankService.TransferMoney.
export const handleTransfer = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const { ticket_v, authenticator, cipher_payload, iv } = req.body;
  try {
    // Decode base64 từ REST body thành bytes đúng contract protobuf.
    const response = await transferMoney({
      ticketV: Buffer.from(ticket_v, "base64"),
      authenticator: Buffer.from(authenticator, "base64"),
      cipherPayload: Buffer.from(cipher_payload, "base64"),
      iv: Buffer.from(iv, "base64"),
    });

    // Chuẩn hóa envelope { success, data } với ap_rep base64 để client giải mã được.
    res.json({
      success: true,
      data: {
        ap_rep: Buffer.from(response.apRep).toString("base64"),
        transaction_id: response.transactionId,
      },
    });
  } catch (err) {
    // Chuyển lỗi gRPC sang errorHandler để map HTTP/error_code tập trung.
    next(err);
  }
};

// Xử lý xem số dư: không cache response và forward ticket/authenticator sang Bank.
export const handleBalanceQuery = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const { ticket_v, authenticator } = req.body;

  // Read path bắt buộc no-store để tránh cache dữ liệu tài khoản.
  res.set("Cache-Control", "no-store");

  try {
    // Forward ticket/authenticator dạng bytes, Gateway không giải mã secret.
    const response = await getBalance({
      ticketV: Buffer.from(ticket_v, "base64"),
      authenticator: Buffer.from(authenticator, "base64"),
      accountId: String(req.params.id),
    });

    // Trả response số dư từ Bank Service cho client.
    res.json(response);
  } catch (err) {
    // Đẩy lỗi về errorHandler chung.
    next(err);
  }
};

// Xử lý xem thông tin cá nhân (/v1/auth/me): forward ticket/authenticator, account_id rỗng
// để Bank tự resolve tài khoản mặc định theo client_id trong ticket. Toàn bộ profile
// (full_name, email, hạn mức, số dư, trạng thái) nằm trong ap_rep mã hóa bằng K_{c,v}.
export const handleProfile = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const { ticket_v, authenticator } = req.body;
  res.set("Cache-Control", "no-store");

  try {
    const response = await getBalance({
      ticketV: Buffer.from(ticket_v, "base64"),
      authenticator: Buffer.from(authenticator, "base64"),
      accountId: "",
    });

    res.json({
      success: true,
      data: {
        ap_rep: Buffer.from(response.apRep).toString("base64"),
        account_id: response.accountId,
      },
    });
  } catch (err) {
    next(err);
  }
};

// Xử lý xem lịch sử giao dịch: parse phân trang và gọi BankService.GetHistory.
export const handleHistoryQuery = async (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const { ticket_v, authenticator } = req.body;
  const { limit, offset } = req.query as Record<string, string>;

  // Không cache lịch sử giao dịch vì đây là dữ liệu nhạy cảm.
  res.set("Cache-Control", "no-store");

  try {
    // Gọi Bank Service lấy lịch sử với account_id và thông tin phân trang.
    const response = await getHistory({
      ticketV: Buffer.from(ticket_v, "base64"),
      authenticator: Buffer.from(authenticator, "base64"),
      accountId: String(req.params.id),
      limit: parseInt(limit ?? "20", 10),
      offset: parseInt(offset ?? "0", 10),
    });

    // Chuẩn hóa envelope { success, data }: ap_rep base64, status enum → domain string,
    // field snake_case để client tiêu thụ trực tiếp. KHÔNG trả client_signature/payload_hash.
    const txnStatus: Record<number, string> = { 1: "pending", 2: "completed", 3: "failed" };
    res.json({
      success: true,
      data: {
        ap_rep: Buffer.from(response.apRep).toString("base64"),
        transactions: response.transactions.map((t) => ({
          transaction_id: t.transactionId,
          from_account_number: t.fromAccountNumber,
          to_account_number: t.toAccountNumber,
          amount: t.amount,
          currency: t.currency,
          status: txnStatus[t.status] ?? "unknown",
          description: t.description,
          scope: t.scope,
          created_at_unix: t.createdAtUnix,
          completed_at_unix: t.completedAtUnix,
        })),
        total: response.total,
        limit: response.limit,
        offset: response.offset,
      },
    });
  } catch (err) {
    // Lỗi gRPC được map ở middleware errorHandler.
    next(err);
  }
};
