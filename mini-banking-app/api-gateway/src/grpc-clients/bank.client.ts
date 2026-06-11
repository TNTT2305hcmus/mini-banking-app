import * as grpc from '@grpc/grpc-js'
import { BankServiceClient } from '../proto/bank'

const addr = process.env.BANK_SERVICE_ADDR || 'localhost:50053'

export const bankClient = new BankServiceClient(addr, grpc.credentials.createInsecure())
