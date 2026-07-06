// Service client cho audit read API: gọi CA.ListAuditEvents và Bank.ListAuditEvents
// qua gRPC TLS, dùng chung credentials với các service client hiện có.
import {
  CAServiceClient,
  ListAuditEventsRequest as CaListAuditRequest,
  ListAuditEventsResponse as CaListAuditResponse,
} from "../proto/ca";
import {
  BankServiceClient,
  ListAuditEventsRequest as BankListAuditRequest,
  ListAuditEventsResponse as BankListAuditResponse,
} from "../proto/bank";
import { sslCredentials } from "../config/grpc";
import ENV from "../config/env";
import { Metadata } from "@grpc/grpc-js";

const caClient = new CAServiceClient(ENV.CA_GRPC_ADDR, sslCredentials);
const bankClient = new BankServiceClient(ENV.BANK_GRPC_ADDR, sslCredentials);

// Trace id của Gateway đi qua gRPC metadata "x-request-id" (transport concern),
// không nằm trong message body — service phía sau tự đọc từ metadata khi cần.
const traceMetadata = (requestId?: string): Metadata => {
  const md = new Metadata();
  if (requestId) md.set("x-request-id", requestId);
  return md;
};

export const listCaAuditEvents = (
  payload: CaListAuditRequest,
  requestId?: string,
): Promise<CaListAuditResponse> =>
  new Promise((resolve, reject) => {
    caClient.listAuditEvents(payload, traceMetadata(requestId), (err, res) => {
      if (err) return reject(err);
      resolve(res);
    });
  });

export const listBankAuditEvents = (
  payload: BankListAuditRequest,
  requestId?: string,
): Promise<BankListAuditResponse> =>
  new Promise((resolve, reject) => {
    bankClient.listAuditEvents(payload, traceMetadata(requestId), (err, res) => {
      if (err) return reject(err);
      resolve(res);
    });
  });
