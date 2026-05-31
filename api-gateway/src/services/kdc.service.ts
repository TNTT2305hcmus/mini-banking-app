import { KDCServiceClient } from "../proto/kdc";
import { sslCredentials } from "../config/grpc";

const kdcServiceClient = new KDCServiceClient(
  "localhost:50053",
  sslCredentials,
);
