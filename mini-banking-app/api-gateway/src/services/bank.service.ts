// Service client cho Bank gRPC: gom mọi lời gọi Bank Service qua một client TLS.
import {
  BalanceRequest,
  BalanceResponse,
  BankServiceClient,
  CreateUserRequest,
  CreateUserResponse,
  HistoryRequest,
  HistoryResponse,
  TransferRequest,
  TransferResponse,
} from "../proto/bank";
import { sslCredentials } from "../config/grpc";
import { Metadata } from "@grpc/grpc-js";

// Địa chỉ Bank Service nội bộ; dùng chung env để tránh lệch config.
const BANK_ADDR = process.env.BANK_GRPC_ADDR ?? "localhost:50053";

// Client gRPC dùng TLS một chiều theo thiết kế ADR-02.
const bankServiceClient = new BankServiceClient(BANK_ADDR, sslCredentials);

// Trace id đi qua gRPC metadata "x-request-id" (transport concern) để Bank audit
// correlate được với cùng request khi AP request_id chưa tồn tại (auth fail sớm).
const traceMetadata = (requestId?: string): Metadata => {
  const md = new Metadata();
  if (requestId) md.set("x-request-id", requestId);
  return md;
};

// Gọi Bank.TransferMoney cho luồng chuyển tiền đã được route validate.
export const transferMoney = (
  payload: TransferRequest,
  requestId?: string,
): Promise<TransferResponse> =>
  new Promise((resolve, reject) => {
    // Gửi Ticket_v, Authenticator và CipherPayload dạng bytes sang Bank.
    bankServiceClient.transferMoney(
      payload,
      traceMetadata(requestId),
      (err, res) => {
        if (err) return reject(err);
        resolve(res);
      },
    );
  });

// Gọi Bank.GetBalance cho read path xem số dư.
export const getBalance = (
  payload: BalanceRequest,
  requestId?: string,
): Promise<BalanceResponse> =>
  new Promise((resolve, reject) => {
    // Bank Service tự giải mã ticket và kiểm tra ownership trước khi trả số dư.
    bankServiceClient.getBalance(
      payload,
      traceMetadata(requestId),
      (err, res) => {
        if (err) return reject(err);
        resolve(res);
      },
    );
  });

// Gọi Bank.GetHistory cho read path xem lịch sử giao dịch.
export const getHistory = (
  payload: HistoryRequest,
  requestId?: string,
): Promise<HistoryResponse> =>
  new Promise((resolve, reject) => {
    // Bank Service áp dụng scope history:read và phân trang theo request.
    bankServiceClient.getHistory(
      payload,
      traceMetadata(requestId),
      (err, res) => {
        if (err) return reject(err);
        resolve(res);
      },
    );
  });

export const createUserBankAccount = (
  payload: CreateUserRequest,
): Promise<CreateUserResponse> =>
  new Promise((resolve, reject) => {
    bankServiceClient.createUser(payload, (err, res) => {
      if (err) reject(err);
      resolve(res);
    });
  });
