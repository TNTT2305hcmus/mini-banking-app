export type OperationId = string;

export function newOperationId(): OperationId {
  return crypto.randomUUID();
}

export function operationHeaders(operationId?: OperationId): Record<string, string> {
  return operationId ? { "X-Request-ID": operationId } : {};
}
