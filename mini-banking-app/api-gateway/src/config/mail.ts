import nodemailer from "nodemailer";
import env from "./env";

const transporter = nodemailer.createTransport({
  host: env.SMTP_HOST,
  port: env.SMTP_PORT,
  secure: false,
  auth: {
    user: env.EMAIL_USER,
    pass: env.EMAIL_PASS,
  },
});

const escapeHtml = (value: string): string =>
  value.replace(
    /[&<>"']/g,
    (character) =>
      ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#039;",
      })[character]!,
  );

export const sendOTPEmail = async (to: string, otp: number) => {
  const options = {
    from: `Mini-Banking System - <no-reply@minibanking.com>`,
    to: to,
    subject: "Mã xác thực đăng ký tài khoản",
    html: `
    <!DOCTYPE html>
    <html>
    <head>
      <meta charset="utf-8">
      <meta name="viewport" content="width=device-width, initial-scale=1.0">
      <title>Xác thực tài khoản</title>
      <style>
        body {
          font-family: Arial, Helvetica, "Times New Roman", sans-serif;
          background-color: #f4f6f8;
          margin: 0;
          padding: 0;
          -webkit-font-smoothing: antialiased;
        }
        .container {
          max-width: 500px;
          margin: 40px auto;
          background: #ffffff;
          border-radius: 12px;
          overflow: hidden;
          box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
          border: 1px solid #e1e8ed;
        }
        .header {
          background-color: #0052cc;
          padding: 30px 20px;
          text-align: center;
        }
        .header h1 {
          color: #ffffff;
          font-size: 24px;
          margin: 0;
          font-weight: bold;
          letter-spacing: 0.5px;
        }
        .content {
          padding: 40px 30px;
          text-align: center;
          color: #333333;
        }
        .content p {
          font-size: 15px;
          line-height: 1.6;
          margin: 0 0 20px 0;
          color: #444444;
        }
        .otp-container {
          background-color: #f4f5f7;
          border-radius: 8px;
          padding: 16px 24px;
          margin: 30px 0;
          display: inline-block;
          border: 1px dashed #0052cc;
        }
        .otp-code {
          font-family: Arial, Helvetica, sans-serif;
          font-size: 36px;
          font-weight: bold;
          letter-spacing: 8px;
          color: #0052cc;
          margin: 0;
        }
        .footer {
          background-color: #fafbfc;
          padding: 20px;
          text-align: center;
          font-size: 12px;
          color: #777777;
          border-top: 1px solid #ebecf0;
        }
        .warning {
          font-size: 13px !important;
          color: #c92a2a !important;
          margin-top: 25px !important;
          font-weight: bold;
        }
      </style>
    </head>
    <body>
      <div class="container">
        <div class="header">
          <h1>Mini-Banking</h1>
        </div>
        
        <div class="content">
          <p>Xin chào,</p>
          <p>Bạn đang thực hiện đăng ký tài khoản tại <strong>Mini-Banking System</strong>. Vui lòng sử dụng mã OTP dưới đây để hoàn tất quá trình xác thực:</p>
          
          <div class="otp-container">
            <h2 class="otp-code">${otp}</h2>
          </div>
          
          <p>Mã này có hiệu lực trong vòng <strong>5 phút</strong>.</p>
          <p class="warning">Tuyệt đối KHÔNG chia sẻ mã này với bất kỳ ai để tránh mất mát tài sản.</p>
        </div>
        
        <div class="footer">
          <p>Đây là email tự động, vui lòng không phản hồi email này.</p>
          <p>&copy; 2026 Mini-Banking System. All rights reserved.</p>
        </div>
      </div>
    </body>
    </html>
    `,
  };

  try {
    const info = await transporter.sendMail(options);
    console.log(`[Email] sent to  ${to}:` + info.messageId);
  } catch (error) {
    console.error(`[Email] failed to send OTP to ${to}:`, error);
    throw new Error(`[Email] Failed to send email to ${to}`);
  }
};

export interface BankAdminActivationEmailInput {
  adminId: string;
  fullName: string;
  expiresAt: string;
  activationUrl: string;
}

export const sendBankAdminActivationEmail = async (
  to: string,
  input: BankAdminActivationEmailInput,
) => {
  const fullName = escapeHtml(input.fullName);
  const adminId = escapeHtml(input.adminId);
  const expiresAt = escapeHtml(input.expiresAt);
  const activationUrl = escapeHtml(input.activationUrl);
  const options = {
    from: `Mini-Banking System - <no-reply@minibanking.com>`,
    to,
    subject: "Kích hoạt tài khoản Bank Admin",
    text: [
      `Xin chào ${input.fullName},`,
      "",
      "Tài khoản Bank Admin của bạn đã được tạo.",
      `Admin ID: ${input.adminId}`,
      `Hết hạn: ${input.expiresAt}`,
      `Liên kết kích hoạt: ${input.activationUrl}`,
      "",
      "Hãy mở liên kết để xác minh thông tin và đặt mã PIN. Không chia sẻ liên kết này.",
    ].join("\n"),
    html: `
      <div style="font-family:Arial,sans-serif;max-width:620px;margin:auto;color:#1f2937">
        <h2 style="color:#0891b2">Kích hoạt Bank Admin</h2>
        <p>Xin chào <strong>${fullName}</strong>,</p>
        <p>Tài khoản Bank Admin của bạn đã được tạo.</p>
        <p><strong>Admin ID:</strong> <code>${adminId}</code></p>
        <p><strong>Hết hạn:</strong> ${expiresAt}</p>
        <p><strong>Liên kết kích hoạt:</strong><br><a href="${activationUrl}" style="word-break:break-all">${activationUrl}</a></p>
        <p>Liên kết sẽ xác minh thông tin Bank Admin và yêu cầu bạn đặt mã PIN.</p>
        <p style="color:#b91c1c"><strong>Không chia sẻ token hoặc liên kết này với bất kỳ ai.</strong></p>
      </div>
    `,
  };

  try {
    const info = await transporter.sendMail(options);
    console.log(`[Email] Bank Admin activation sent to ${to}: ${info.messageId}`);
  } catch (error) {
    console.error(`[Email] failed to send Bank Admin activation to ${to}:`, error);
    throw new Error(`[Email] Failed to send Bank Admin activation to ${to}`);
  }
};
