import {
  CAServiceClient,
  RegisterUserRequest,
  RegisterUserResponse,
} from "../proto/ca";
import { sslCredentials } from "../config/grpc";
import ENV from "../config/env";
import { Metadata } from "@grpc/grpc-js";
import { UUID } from "node:crypto";

const caClient = new CAServiceClient(ENV.CA_GRPC_ADDR, sslCredentials);

export const registerUser = (
  payload: RegisterUserRequest,
): Promise<RegisterUserResponse> => {
  return new Promise((resolve, reject) => {
    {
      caClient.registerUser(payload, (err, res) => {
        if (err) reject(err);
        else resolve(res);
      });
    }
  });
};
