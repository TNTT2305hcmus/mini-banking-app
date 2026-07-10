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

interface ActivationEmailTheme {
  roleName: string;
  title: string;
  headerBackground: string;
  pageBackground: string;
  accent: string;
  badgeBackground: string;
  border: string;
}

const buildActivationEmail = (
  input: BankAdminActivationEmailInput,
  theme: ActivationEmailTheme,
) => {
  const fullName = escapeHtml(input.fullName);
  const adminId = escapeHtml(input.adminId);
  const expiresAt = escapeHtml(input.expiresAt);
  const activationUrl = escapeHtml(input.activationUrl);

  return {
    text: [
      `Xin chào ${input.fullName},`,
      "",
      `Tài khoản ${theme.roleName} của bạn đã được tạo.`,
      `Admin ID: ${input.adminId}`,
      `Hết hạn: ${input.expiresAt}`,
      `Liên kết kích hoạt: ${input.activationUrl}`,
      "",
      "Hãy mở liên kết để xác minh thông tin và đặt mã PIN. Không chia sẻ liên kết này.",
    ].join("\n"),
    html: `
    <!DOCTYPE html>
    <html>
    <head>
      <meta charset="utf-8">
      <meta name="viewport" content="width=device-width, initial-scale=1.0">
      <title>${theme.title}</title>
      <style>
        body {
          font-family: Arial, Helvetica, "Times New Roman", sans-serif;
          background-color: ${theme.pageBackground};
          margin: 0;
          padding: 0;
          -webkit-font-smoothing: antialiased;
        }
        .container {
          max-width: 560px;
          margin: 40px auto;
          background: #ffffff;
          border-radius: 12px;
          overflow: hidden;
          box-shadow: 0 10px 30px rgba(15, 23, 42, 0.12);
          border: 1px solid ${theme.border};
        }
        .header {
          background-color: ${theme.headerBackground};
          padding: 32px 24px;
          text-align: center;
        }
        .header .eyebrow {
          color: rgba(255, 255, 255, 0.78);
          font-size: 12px;
          font-weight: bold;
          letter-spacing: 1.6px;
          text-transform: uppercase;
          margin: 0 0 8px 0;
        }
        .header h1 {
          color: #ffffff;
          font-size: 24px;
          margin: 0;
          font-weight: bold;
          letter-spacing: 0.3px;
        }
        .content {
          padding: 36px 30px;
          color: #263238;
        }
        .content p {
          font-size: 15px;
          line-height: 1.65;
          margin: 0 0 18px 0;
          color: #374151;
        }
        .identity-card {
          background-color: ${theme.badgeBackground};
          border: 1px solid ${theme.border};
          border-radius: 10px;
          padding: 18px 20px;
          margin: 24px 0;
        }
        .identity-row {
          margin: 0 0 10px 0;
          font-size: 14px;
          color: #374151;
        }
        .identity-row:last-child {
          margin-bottom: 0;
        }
        .label {
          display: inline-block;
          min-width: 86px;
          color: #6b7280;
          font-weight: bold;
        }
        code {
          font-family: Consolas, Menlo, Monaco, monospace;
          color: #111827;
          word-break: break-all;
        }
        .button-wrap {
          text-align: center;
          margin: 28px 0 20px 0;
        }
        .button {
          display: inline-block;
          background-color: ${theme.accent};
          color: #ffffff !important;
          text-decoration: none;
          border-radius: 8px;
          padding: 14px 22px;
          font-size: 15px;
          font-weight: bold;
        }
        .raw-link {
          font-size: 12px !important;
          color: #6b7280 !important;
          word-break: break-all;
          margin-top: 18px !important;
        }
        .warning {
          font-size: 13px !important;
          color: #b91c1c !important;
          font-weight: bold;
          margin-top: 24px !important;
        }
        .footer {
          background-color: #f8fafc;
          padding: 20px;
          text-align: center;
          font-size: 12px;
          color: #777777;
          border-top: 1px solid #e5e7eb;
        }
      </style>
    </head>
    <body>
      <div class="container">
        <div class="header">
          <p class="eyebrow">Mini Banking Privileged Access</p>
          <h1>${theme.title}</h1>
        </div>

        <div class="content">
          <p>Xin chào <strong>${fullName}</strong>,</p>
          <p>Tài khoản <strong>${theme.roleName}</strong> của bạn đã được tạo. Vui lòng mở liên kết bên dưới để xác minh thông tin và đặt mã PIN cho chứng chỉ quản trị.</p>

          <div class="identity-card">
            <p class="identity-row"><span class="label">Vai trò</span> ${theme.roleName}</p>
            <p class="identity-row"><span class="label">Admin ID</span> <code>${adminId}</code></p>
            <p class="identity-row"><span class="label">Hết hạn</span> ${expiresAt}</p>
          </div>

          <div class="button-wrap">
            <a class="button" href="${activationUrl}">Mở liên kết kích hoạt</a>
          </div>

          <p class="raw-link">Nếu nút không mở được, hãy copy liên kết này vào trình duyệt:<br>${activationUrl}</p>
          <p class="warning">Không chia sẻ token hoặc liên kết này với bất kỳ ai.</p>
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
};

const bankAdminTheme: ActivationEmailTheme = {
  roleName: "Bank Admin",
  title: "Kích hoạt Bank Admin",
  headerBackground: "#075985",
  pageBackground: "#e0f2fe",
  accent: "#0284c7",
  badgeBackground: "#f0f9ff",
  border: "#bae6fd",
};

const caAdminTheme: ActivationEmailTheme = {
  roleName: "CA Admin",
  title: "Kích hoạt CA Admin",
  headerBackground: "#0f766e",
  pageBackground: "#ccfbf1",
  accent: "#0d9488",
  badgeBackground: "#f0fdfa",
  border: "#99f6e4",
};

export const sendBankAdminActivationEmail = async (
  to: string,
  input: BankAdminActivationEmailInput,
) => {
  const email = buildActivationEmail(input, bankAdminTheme);
  const options = {
    from: `Mini-Banking System - <no-reply@minibanking.com>`,
    to,
    subject: "Kích hoạt tài khoản Bank Admin",
    text: email.text,
    html: email.html,
  };

  try {
    const info = await transporter.sendMail(options);
    console.log(`[Email] Bank Admin activation sent to ${to}: ${info.messageId}`);
  } catch (error) {
    console.error(`[Email] failed to send Bank Admin activation to ${to}:`, error);
    throw new Error(`[Email] Failed to send Bank Admin activation to ${to}`);
  }
};

export const sendCaAdminActivationEmail = async (
  to: string,
  input: BankAdminActivationEmailInput,
) => {
  const email = buildActivationEmail(input, caAdminTheme);
  const options = {
    from: `Mini-Banking System - <no-reply@minibanking.com>`,
    to,
    subject: "Kích hoạt tài khoản CA Admin",
    text: email.text,
    html: email.html,
  };

  try {
    const info = await transporter.sendMail(options);
    console.log(`[Email] CA Admin activation sent to ${to}: ${info.messageId}`);
  } catch (error) {
    console.error(`[Email] failed to send CA Admin activation to ${to}:`, error);
    throw new Error(`[Email] Failed to send CA Admin activation to ${to}`);
  }
};
