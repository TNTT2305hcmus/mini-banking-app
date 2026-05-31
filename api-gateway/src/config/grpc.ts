import * as grpc from "@grpc/grpc-js";
import * as fs from "fs";
import ENV from "./env";

const caCert = fs.readFileSync(ENV.CA_CERT_PATH);
const clientKey = fs.readFileSync(ENV.PRIVATE_KEY_PATH);
const clientCert = fs.readFileSync(ENV.GATEWAY_CERT_PATH);

export const sslCredentials = grpc.credentials.createSsl(
  caCert,
  clientKey,
  clientCert,
);
