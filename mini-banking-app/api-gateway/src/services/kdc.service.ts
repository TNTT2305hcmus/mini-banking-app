import { ASRequest, ASResponse, KDCServiceClient, TGSRequest, TGSResponse } from "../proto/kdc";
import { sslCredentials } from "../config/grpc";

const KDC_ADDR = process.env.KDC_GRPC_ADDR ?? "localhost:50052";

const kdcClient = new KDCServiceClient(KDC_ADDR, sslCredentials);

export const requestTgt = (payload: ASRequest): Promise<ASResponse> =>
  new Promise((resolve, reject) => {
    kdcClient.requestTgt(payload, (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });

export const requestServiceTicket = (payload: TGSRequest): Promise<TGSResponse> =>
  new Promise((resolve, reject) => {
    kdcClient.requestServiceTicket(payload, (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });
