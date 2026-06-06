import { Request, Response, NextFunction } from "express";

export const errorHandler = (err: Error, _req: Request, res: Response, _next: NextFunction) => {
  console.error("[UNHANDLED]", err);
  res.status(500).json({
    success: false,
    error_code: "INTERNAL_ERROR",
    message: "An unexpected error occurred",
  });
};
