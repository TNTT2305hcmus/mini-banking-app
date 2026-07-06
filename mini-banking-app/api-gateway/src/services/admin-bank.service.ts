import {
  BankServiceClient,
  type AdminOverviewRequest,
  type AdminOverviewResponse,
  type CreateAdminSessionRequest,
  type CreateAdminSessionResponse,
  type ListAdminAuditEventsRequest,
  type ListAdminAuditEventsResponse,
  type ListAdminTransactionsRequest,
  type ListAdminTransactionsResponse,
  type ListAdminUserAccountsRequest,
  type ListAdminUserAccountsResponse,
  type ListAdminUsersRequest,
  type ListAdminUsersResponse,
} from "../proto/bank";
import { CAServiceClient, IdentityRole, type RegisterUserResponse } from "../proto/ca";
import { sslCredentials } from "../config/grpc";
import ENV from "../config/env";

const caClient = new CAServiceClient(ENV.CA_GRPC_ADDR, sslCredentials);
const bankClient = new BankServiceClient(
  process.env.BANK_GRPC_ADDR ?? "localhost:50053",
  sslCredentials,
);

export interface IssueBankAdminCertificateInput {
  csrPem: string;
  adminId: string;
  subjectEmail: string;
  fullName: string;
}

// Role is fixed here so neither the controller nor frontend can choose it.
export const issueBankAdminCertificate = (
  input: IssueBankAdminCertificateInput,
): Promise<RegisterUserResponse> =>
  new Promise((resolve, reject) => {
    caClient.registerUser(
      {
        csrPem: input.csrPem,
        ownerId: input.adminId,
        subjectEmail: input.subjectEmail,
        fullName: input.fullName,
        role: IdentityRole.IDENTITY_ROLE_BANK_ADMIN,
      },
      (err, response) => {
        if (err) reject(err);
        else resolve(response);
      },
    );
  });

const unary = <Request, Response>(
  invoke: (
    request: Request,
    callback: (error: Error | null, response: Response) => void,
  ) => void,
  request: Request,
): Promise<Response> =>
  new Promise((resolve, reject) => {
    invoke(request, (error, response) => {
      if (error) reject(error);
      else resolve(response);
    });
  });

export const createBankAdminSession = (
  request: CreateAdminSessionRequest,
): Promise<CreateAdminSessionResponse> =>
  unary(bankClient.createAdminSession.bind(bankClient), request);

export const getBankAdminOverview = (
  request: AdminOverviewRequest,
): Promise<AdminOverviewResponse> =>
  unary(bankClient.getAdminOverview.bind(bankClient), request);

export const listBankAdminUsers = (
  request: ListAdminUsersRequest,
): Promise<ListAdminUsersResponse> =>
  unary(bankClient.listAdminUsers.bind(bankClient), request);

export const listBankAdminUserAccounts = (
  request: ListAdminUserAccountsRequest,
): Promise<ListAdminUserAccountsResponse> =>
  unary(bankClient.listAdminUserAccounts.bind(bankClient), request);

export const listBankAdminTransactions = (
  request: ListAdminTransactionsRequest,
): Promise<ListAdminTransactionsResponse> =>
  unary(bankClient.listAdminTransactions.bind(bankClient), request);

export const listBankAdminAuditEvents = (
  request: ListAdminAuditEventsRequest,
): Promise<ListAdminAuditEventsResponse> =>
  unary(bankClient.listAdminAuditEvents.bind(bankClient), request);
