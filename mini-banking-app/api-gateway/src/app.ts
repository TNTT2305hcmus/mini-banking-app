import express from 'express'
import { bankRouter } from './routes/bank.routes'
import { rateLimit } from './middleware/rate-limit'
import { errorEnvelope } from './middleware/error-envelope'

const app = express()

app.use(express.json())
app.use(rateLimit({ windowMs: 60_000, max: 60 }))

app.get('/health', (_req, res) => {
  res.json({ status: 'ok' })
})

app.use('/v1', bankRouter)

app.use(errorEnvelope)

const port = process.env.PORT || 3000
app.listen(port, () => {
  console.log(`[Gateway] listening on :${port}`)
})

process.on('SIGTERM', () => {
  console.log('[Gateway] SIGTERM received, shutting down')
  process.exit(0)
})
