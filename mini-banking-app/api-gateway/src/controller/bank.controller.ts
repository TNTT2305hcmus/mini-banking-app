import { Request, Response } from "express";

const handleTransferMoney = (req: Request, res: Response) => {
  const { ticketV, authenticator, cipherPayload, iv } = req.body;
};
