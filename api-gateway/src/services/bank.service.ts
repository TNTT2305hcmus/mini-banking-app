import { BankServiceClient } from "../proto/bank";
import { sslCredentials } from "../config/grpc";

const BANK_ADDR = process.env.BANK_GRPC_ADDR ?? "localhost:50053";

const bankServiceClient = new BankServiceClient(BANK_ADDR, sslCredentials);
