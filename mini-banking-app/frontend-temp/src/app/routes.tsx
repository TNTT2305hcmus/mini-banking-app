import { createBrowserRouter, Navigate } from "react-router"
import Register from "../pages/Register"
import Login from "../pages/Login"
import Home from "../pages/Home"
import AdminCA from "../pages/AdminCA"
import AdminBank from "../pages/AdminBank"

export const router = createBrowserRouter([
  { index: true, element: <Navigate to="/login" replace /> },
  { path: "/register", Component: Register },
  { path: "/login", Component: Login },
  { path: "/home", Component: Home },
  { path: "/admin-ca", Component: AdminCA },
  { path: "/admin-bank", Component: AdminBank },
  { path: "*", element: <Navigate to="/login" replace /> },
])
