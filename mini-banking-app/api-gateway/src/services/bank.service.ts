import {
  BankServiceClient,
  CreateUserRequest,
  CreateUserResponse,
} from "../proto/bank";
import { sslCredentials } from "../config/grpc";

const BANK_ADDR = process.env.BANK_GRPC_ADDR ?? "localhost:50053";

const bankServiceClient = new BankServiceClient(BANK_ADDR, sslCredentials);

export const createUser = (
  payload: CreateUserRequest,
): Promise<CreateUserResponse> =>
  new Promise((resolve, reject) => {
    bankServiceClient.createUser(payload, (err, res) => {
      if (err) reject(err);
      resolve(res);
    });
  });
