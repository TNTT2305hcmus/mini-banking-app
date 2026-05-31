import { BankServiceClient } from "../proto/bank";
import { sslCredentials } from "../config/grpc";

const bankServiceClient = new BankServiceClient(
  "localhost:50051",
  sslCredentials,
);
