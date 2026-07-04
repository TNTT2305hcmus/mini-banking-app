import { createBrowserRouter, Navigate } from "react-router"
import Register from "../pages/Register"
import Login from "../pages/Login"
import Home from "../pages/Home"
import AdminCA from "../pages/AdminCA"
import AdminBank from "../pages/AdminBank"
import AdminBankActivate from "../pages/AdminBankActivate"
import AdminBankLogin from "../pages/AdminBankLogin"

export const router = createBrowserRouter([
  { index: true, element: <Navigate to="/login" replace /> },
  { path: "/register", Component: Register },
  { path: "/login", Component: Login },
  { path: "/home", Component: Home },
  { path: "/admin-ca", Component: AdminCA },
  { path: "/admin-bank", Component: AdminBank },
  { path: "/admin-bank/activate", Component: AdminBankActivate },
  { path: "/admin-bank/login", Component: AdminBankLogin },
  { path: "*", element: <Navigate to="/login" replace /> },
])
