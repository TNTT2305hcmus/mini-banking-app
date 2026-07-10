import { createBrowserRouter, Navigate } from "react-router"
import Register from "../pages/Register"
import Login from "../pages/Login"
import Home from "../pages/Home"
import AdminCA from "../pages/AdminCA"
import AdminCAActivate from "../pages/AdminCAActivate"
import AdminBank from "../pages/AdminBank"
import AdminBankActivate from "../pages/AdminBankActivate"
import AdminSOC from "../pages/AdminSOC"

export const router = createBrowserRouter([
  { index: true, element: <Navigate to="/login" replace /> },
  { path: "/register", Component: Register },
  { path: "/login", Component: Login },
  { path: "/home", Component: Home },
  { path: "/admin-ca", Component: AdminCA },
  { path: "/admin-ca/activate", Component: AdminCAActivate },
  { path: "/admin-bank", Component: AdminBank },
  { path: "/admin-bank/activate", Component: AdminBankActivate },
  { path: "/admin-bank/login", element: <Navigate to="/admin-bank" replace /> },
  { path: "/admin-soc", Component: AdminSOC },
  { path: "*", element: <Navigate to="/login" replace /> },
])
