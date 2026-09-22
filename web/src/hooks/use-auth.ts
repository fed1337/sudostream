import { useQuery } from "@tanstack/react-query";

import { getApiAuthMeOptions, getApiAuthMeQueryKey } from "@/client/@tanstack/react-query.gen";
import { postApiAuthLogout } from "@/client/sdk.gen";
import { clearAccessToken, getAccessToken, setAccessToken } from "@/lib/auth-storage";

type UseCurrentUserOptions = {
  enabled?: boolean;
};

export const currentUserQueryKey = getApiAuthMeQueryKey();

export function useCurrentUser(options?: UseCurrentUserOptions) {
  const enabled = options?.enabled ?? Boolean(getAccessToken());

  return useQuery({
    ...getApiAuthMeOptions(),
    enabled,
    retry: false,
    staleTime: 60_000,
  });
}

export function invalidateCurrentUserQuery(queryClient: {
  invalidateQueries: (options: {
    queryKey: ReturnType<typeof getApiAuthMeQueryKey>;
  }) => Promise<unknown>;
}): Promise<unknown> {
  return queryClient.invalidateQueries({ queryKey: getApiAuthMeQueryKey() });
}

export function useLogout() {
  return async () => {
    try {
      await postApiAuthLogout();
    } finally {
      clearAccessToken();
      window.location.href = "/login";
    }
  };
}

export function persistLoginAccessToken(accessToken: string): void {
  setAccessToken(accessToken);
}

export function completeLoginSession(
  queryClient: {
    invalidateQueries: (options: {
      queryKey: ReturnType<typeof getApiAuthMeQueryKey>;
    }) => Promise<unknown>;
    setQueryData?: (queryKey: ReturnType<typeof getApiAuthMeQueryKey>, data: unknown) => void;
  },
  accessToken: string,
  user?: { id?: string; email?: string; role?: string; mustChangePassword?: boolean },
): void {
  persistLoginAccessToken(accessToken);
  if (user && queryClient.setQueryData) {
    queryClient.setQueryData(getApiAuthMeQueryKey(), { user });
  }
  void invalidateCurrentUserQuery(queryClient);
}

export function useIsAdmin(): boolean {
  const { data } = useCurrentUser();
  return data?.user?.role === "admin";
}
