import { Metadata } from "@grpc/grpc-js";
import { sslCredentials } from "../config/grpc";
import ENV from "../config/env";
import {
  CAServiceClient,
  GetCertificateDetailRequest,
  GetCertificateDetailResponse,
  ListCertificatesRequest,
  ListCertificatesResponse,
  RegisterUserRequest,
  RegisterUserResponse,
  RevokeCertificateRequest,
  RevokeCertificateResponse,
  VerifyCertificateRequest,
  VerifyCertificateResponse,
} from "../proto/ca";

const caServiceClient = new CAServiceClient(ENV.CA_GRPC_ADDR, sslCredentials);

const requestMetadata = (requestId?: string) => {
  const metadata = new Metadata();
  if (requestId) {
    metadata.set("x-request-id", requestId);
  }
  return metadata;
};

export const registerUser = (
  payload: RegisterUserRequest,
  requestId?: string,
): Promise<RegisterUserResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.registerUser(payload, requestMetadata(requestId), (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });

export const verifyCertificate = (
  payload: VerifyCertificateRequest,
): Promise<VerifyCertificateResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.verifyCertificate(payload, (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });

export const listCertificates = (
  payload: ListCertificatesRequest,
  requestId?: string,
): Promise<ListCertificatesResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.listCertificates(payload, requestMetadata(requestId), (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });

export const getCertificateDetail = (
  payload: GetCertificateDetailRequest,
  requestId?: string,
): Promise<GetCertificateDetailResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.getCertificateDetail(payload, requestMetadata(requestId), (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });

export const revokeCertificate = (
  payload: RevokeCertificateRequest,
  requestId?: string,
): Promise<RevokeCertificateResponse> =>
  new Promise((resolve, reject) => {
    caServiceClient.revokeCertificate(payload, requestMetadata(requestId), (err, res) => {
      if (err) reject(err);
      else resolve(res);
    });
  });
