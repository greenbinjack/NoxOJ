import { Navigate, Outlet } from "react-router-dom";
import { useAuth } from "./AuthContext";

// Guards a route group behind authentication. While the initial
// /users/me check is in flight, render nothing rather than briefly
// flashing a login redirect for a user who turns out to be logged in.
export function ProtectedRoute() {
  const { user, loading } = useAuth();

  if (loading) return null;
  if (!user) return <Navigate to="/login" replace />;

  return <Outlet />;
}
