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

const BANK_ADDR = process.env.BANK_GRPC_ADDR ?? "localhost:50053";

const bankServiceClient = new BankServiceClient(BANK_ADDR, sslCredentials);

export const createUser = (
  payload: CreateUserRequest,
): Promise<CreateUserResponse> =>
  new Promise((resolve, reject) => {
    bankServiceClient.createUser(payload, (err, res) => {
      if (err) return reject(err);
      resolve(res);
    });
  });

export const transferMoney = (
  payload: TransferRequest,
): Promise<TransferResponse> =>
  new Promise((resolve, reject) => {
    bankServiceClient.transferMoney(payload, (err, res) => {
      if (err) return reject(err);
      resolve(res);
    });
  });

export const getBalance = (
  payload: BalanceRequest,
): Promise<BalanceResponse> =>
  new Promise((resolve, reject) => {
    bankServiceClient.getBalance(payload, (err, res) => {
      if (err) return reject(err);
      resolve(res);
    });
  });

export const getHistory = (
  payload: HistoryRequest,
): Promise<HistoryResponse> =>
  new Promise((resolve, reject) => {
    bankServiceClient.getHistory(payload, (err, res) => {
      if (err) return reject(err);
      resolve(res);
    });
  });
