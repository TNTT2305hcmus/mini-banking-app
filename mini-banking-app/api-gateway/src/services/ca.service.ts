import {
  CAServiceClient,
  RegisterUserRequest,
  RegisterUserResponse,
} from "../proto/ca";
import { sslCredentials } from "../config/grpc";
import ENV from "../config/env";
import { Metadata } from "@grpc/grpc-js";

const caClient = new CAServiceClient(ENV.CA_GRPC_ADDR, sslCredentials);

export const registerUser = (
  payload: RegisterUserRequest,
  requestId?: string,
): Promise<RegisterUserResponse> =>
  new Promise((resolve, reject) => {
    // Trace id đi qua gRPC metadata "x-request-id"; CA đọc nó để ghi vào
    // metadata của audit event `issued`.
    const md = new Metadata();
    if (requestId) md.set("x-request-id", requestId);
    caClient.registerUser(payload, md, (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });
