import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";

export function ProfilePage() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  // ProtectedRoute already guarantees user is non-null before this
  // page renders — this check exists only to satisfy TypeScript.
  if (!user) return null;

  async function handleLogout() {
    await logout();
    navigate("/login");
  }

  return (
    <main>
      <h1>{user.display_name}</h1>
      <dl>
        <dt>Username</dt>
        <dd>@{user.username}</dd>
        <dt>Rating</dt>
        <dd>{user.rating}</dd>
        <dt>Roles</dt>
        <dd>{user.roles.join(", ") || "none"}</dd>
      </dl>
      <button onClick={handleLogout}>Log out</button>
    </main>
  );
}
