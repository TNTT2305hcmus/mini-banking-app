import {
  BalanceRequest,
  BalanceResponse,
  BankServiceClient,
  HistoryRequest,
  HistoryResponse,
  TransferRequest,
  TransferResponse,
} from "../proto/bank";
import { sslCredentials } from "../config/grpc";

const BANK_ADDR = process.env.BANK_GRPC_ADDR ?? "localhost:50053";

const bankServiceClient = new BankServiceClient(BANK_ADDR, sslCredentials);

const transferMoney = (payload: TransferRequest): Promise<TransferResponse> =>
  new Promise((resolve, reject) => {
    bankServiceClient.transferMoney(payload, (err, res) => {
      if (err) reject(err);
      resolve(res);
    });
  });

const getBalance = (payload: BalanceRequest): Promise<BalanceResponse> =>
  new Promise((resolve, reject) => {
    bankServiceClient.getBalance(payload, (err, res) => {
      if (err) reject(err);
      resolve(res);
    });
  });

const getTransactions = (payload: HistoryRequest): Promise<HistoryResponse> =>
  new Promise((resolve, reject) => {
    bankServiceClient.getHistory(payload, (err, res) => {
      if (err) reject(err);
      resolve(res);
    });
  });
