import { Request, Response, NextFunction } from "express";

export const validateHeaders = (
  req: Request,
  res: Response,
  next: NextFunction,
) => {
  const requestId = req.headers["x-request-id"];
  if (!requestId) {
    return res.status(400).json({
      success: false,
      error_code: "MISSING_REQUEST_ID",
      message: "X-Request-ID header is required",
    });
  }
  next();
};
