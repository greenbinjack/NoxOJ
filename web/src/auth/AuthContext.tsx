import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { get, post } from "../api/client";

export interface User {
  id: string;
  username: string;
  email?: string;
  display_name: string;
  rating: number;
  roles: string[];
}

interface AuthContextValue {
  user: User | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

// The access token is an httpOnly cookie — this app can never read it
// directly (that's the point, see Sprint 9). So auth state isn't
// "check if a token exists," it's "ask the backend who I am right
// now" via GET /users/me: 200 means logged in, 401 means not. The
// backend is the single source of truth; this just reflects it.
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const refetch = useCallback(async () => {
    try {
      const me = await get<User>("/users/me");
      setUser(me);
    } catch {
      setUser(null);
    }
  }, []);

  useEffect(() => {
    refetch().finally(() => setLoading(false));
  }, [refetch]);

  const login = useCallback(
    async (username: string, password: string) => {
      await post("/login", { username, password });
      // The login response is intentionally minimal (id + username) —
      // refetch the canonical profile shape from /users/me instead of
      // duplicating it in two different response shapes.
      await refetch();
    },
    [refetch],
  );

  const logout = useCallback(async () => {
    await post("/logout");
    setUser(null);
  }, []);

  return (
    <AuthContext.Provider value={{ user, loading, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within an AuthProvider");
  return ctx;
}
