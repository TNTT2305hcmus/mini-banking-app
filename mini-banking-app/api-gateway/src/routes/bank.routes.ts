import { Router, Request, Response, NextFunction } from 'express'
import { bankClient } from '../grpc-clients/bank.client'
import { CreateUserRequest } from '../proto/bank'
import { requireBodyFields, requireQueryFields } from '../middleware/validate'

export const bankRouter = Router()

bankRouter.post(
  '/bank/transfer',
  requireBodyFields('ticket_v', 'authenticator', 'cipher_payload', 'iv', 'request_id'),
  (req: Request, res: Response, next: NextFunction) => {
    const { ticket_v, authenticator, cipher_payload, iv, request_id } = req.body
    bankClient.transferMoney(
      {
        ticketV: Buffer.from(ticket_v, 'base64'),
        authenticator: Buffer.from(authenticator, 'base64'),
        cipherPayload: Buffer.from(cipher_payload, 'base64'),
        iv: Buffer.from(iv, 'base64'),
        requestId: request_id,
      },
      (err, response) => {
        if (err) return next(err)
        res.json(response)
      },
    )
  },
)

bankRouter.get(
  '/bank/accounts/:id/balance/query',
  requireQueryFields('ticket_v', 'authenticator', 'request_id'),
  (req: Request, res: Response, next: NextFunction) => {
    const { ticket_v, authenticator, request_id } = req.query as Record<string, string>
    bankClient.getBalance(
      {
        ticketV: Buffer.from(ticket_v, 'base64'),
        authenticator: Buffer.from(authenticator, 'base64'),
        accountId: String(req.params.id),
        requestId: request_id,
      },
      (err, response) => {
        if (err) return next(err)
        res.json(response)
      },
    )
  },
)

bankRouter.get(
  '/bank/accounts/:id/transactions/query',
  requireQueryFields('ticket_v', 'authenticator', 'request_id'),
  (req: Request, res: Response, next: NextFunction) => {
    const { ticket_v, authenticator, request_id, limit, offset } = req.query as Record<string, string>
    bankClient.getHistory(
      {
        ticketV: Buffer.from(ticket_v, 'base64'),
        authenticator: Buffer.from(authenticator, 'base64'),
        accountId: String(req.params.id),
        limit: parseInt(limit ?? '20', 10),
        offset: parseInt(offset ?? '0', 10),
        requestId: request_id,
      },
      (err, response) => {
        if (err) return next(err)
        res.json(response)
      },
    )
  },
)

export function callCreateUser(req: CreateUserRequest): Promise<void> {
  return new Promise((resolve, reject) => {
    bankClient.createUser(req, (err) => {
      if (err) return reject(err)
      resolve()
    })
  })
}
